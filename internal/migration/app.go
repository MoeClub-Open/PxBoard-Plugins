package migration

import (
	"context"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type app struct {
	*c.App
	Source, ID, State string
	mu                sync.Mutex
	running           map[string]bool
}

func New(source string) http.Handler {
	a := &app{App: c.New(), Source: source, ID: "import-" + source, State: os.Getenv("PX_PLUGIN_STATE_DIR"), running: map[string]bool{}}
	a.C.HTTP.Timeout = 15 * time.Minute
	a.Mux.HandleFunc("GET /jobs", a.jobs)
	a.Mux.HandleFunc("POST /jobs", a.create)
	a.Mux.HandleFunc("POST /jobs/{id}/{action}", a.action)
	a.Mux.HandleFunc("GET /archive", a.archive)
	a.View("migration", a.view)
	return a
}
func (a *app) jobs(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("jobs")
	for _, row := range rows {
		delete(row, "dsn")
		delete(row, "passwordSalt")
		delete(row, "seed")
	}
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) create(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if (p.Text(b, "sourcePath") == "") == (p.Text(b, "dsn") == "") {
		p.Fail(w, 400, "请选择快照文件或数据库连接其中一种方式")
		return
	}
	if p.Text(b, "dsn") != "" && !sourcePattern.MatchString(p.Text(b, "sourceId")) {
		p.Fail(w, 400, "数据库导入需要唯一的来源标识（小写字母、数字、下划线或连字符）")
		return
	}
	if a.State == "" {
		p.Fail(w, 400, "请升级核心以提供插件持久状态目录")
		return
	}
	b["id"] = p.ID()
	b["seed"] = p.ID() + p.ID()
	b["status"] = "created"
	b["createdAt"] = c.Now()
	b["updatedAt"] = c.Now()
	b["report"] = "尚未预览"
	if !c.Err(w, a.C.Put("jobs", b)) {
		p.JSON(w, p.Record{"id": b["id"], "message": "任务已创建，请先预览"})
	}
}
func (a *app) action(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	job, e := a.Find("jobs", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	action := r.PathValue("action")
	if action == "report" {
		p.JSON(w, p.Record{"title": "迁移报告", "details": job["report"]})
		return
	}
	if action != "preview" && action != "apply" {
		p.Fail(w, 404, "操作不存在")
		return
	}
	if p.Text(job, "status") == "committed" {
		p.JSON(w, p.Record{"message": "该任务已导入，本次没有重复写入"})
		return
	}
	if action == "apply" && p.Text(job, "status") != "ready" {
		p.Fail(w, 400, "请先完成预览并解决阻断问题")
		return
	}
	id := p.Text(job, "id")
	a.mu.Lock()
	if a.running[id] {
		a.mu.Unlock()
		p.Fail(w, 409, "任务仍在运行，请稍后查看状态")
		return
	}
	a.running[id] = true
	a.mu.Unlock()
	delete(job, "error")
	if action == "preview" {
		job["status"] = "preparing"
	} else {
		job["status"] = "importing"
	}
	job["updatedAt"] = c.Now()
	if e = a.C.Put("jobs", job); c.Err(w, e) {
		a.mu.Lock()
		delete(a.running, id)
		a.mu.Unlock()
		return
	}
	go a.work(job, action)
	p.JSON(w, p.Record{"message": "任务已启动，可查看状态与迁移报告"})
}
func (a *app) work(job p.Record, action string) {
	id := p.Text(job, "id")
	defer func() { a.mu.Lock(); delete(a.running, id); a.mu.Unlock() }()
	var e error
	if action == "preview" {
		e = a.prepare(job)
	} else {
		var report p.ImportReport
		e = a.C.Request("POST", "/api/v1/runtime/import/"+id+"/commit", p.Record{}, &report)
		if e == nil {
			job["status"] = report.Status
			job["errorCount"] = len(report.Errors)
			if len(report.Errors) > 0 {
				job["report"] = p.Text(job, "report") + "\n\n目标检查阻断：\n" + strings.Join(report.Errors, "\n")
			}
		}
	}
	if e != nil {
		job["status"] = "failed"
		job["error"] = e.Error()
		job["report"] = p.Text(job, "report") + "\n\n错误：" + e.Error()
	} else {
		delete(job, "error")
	}
	job["updatedAt"] = c.Now()
	_ = a.C.Put("jobs", job)
}
func (a *app) prepare(job p.Record) error {
	id := p.Text(job, "id")
	if e := os.MkdirAll(a.State, 0700); e != nil {
		return e
	}
	path := filepath.Join(a.State, id+".snapshot.json")
	snapshot, e := LoadSnapshot(path)
	if os.IsNotExist(e) {
		if p.Text(job, "sourcePath") != "" {
			snapshot, e = LoadSnapshot(p.Text(job, "sourcePath"))
		} else {
			zone := p.Text(job, "timezone")
			if zone == "" {
				zone = "Asia/Shanghai"
			}
			snapshot, e = Export(context.Background(), p.Text(job, "dsn"), a.Source, p.Text(job, "sourceId"), zone)
		}
		if e != nil {
			return e
		}
		if snapshot.Source != a.Source {
			return fmt.Errorf("快照来源为 %s，请使用对应的迁移插件", snapshot.Source)
		}
		if e = SaveSnapshot(path, snapshot); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	job["sourceId"] = snapshot.SourceID
	job["snapshotPath"] = path
	delete(job, "dsn")
	plan, e := Build(snapshot, a.ID, Options{IncludeNodes: p.Bool(job, "includeNodes"), IncludeAffiliate: p.Bool(job, "includeAffiliate"), IncludeRoutes: p.Bool(job, "includeRoutes"), IncludeTemplates: p.Bool(job, "includeTemplates"), PasswordMethod: p.Text(job, "passwordMethod"), PasswordSalt: p.Text(job, "passwordSalt"), SystemResetMethod: p.Text(job, "systemResetMethod"), Seed: p.Text(job, "seed")})
	if e != nil {
		return e
	}
	job["sourceRows"] = plan.SourceRows
	job["archiveRows"] = plan.ArchiveRows
	mapped := 0
	for _, count := range plan.Mapped {
		mapped += count
	}
	job["mappedRows"] = mapped
	job["warningCount"] = len(plan.Warnings)
	job["errorCount"] = len(plan.Errors)
	job["report"] = reportText(plan)
	job["tables"] = snapshot.Names()
	if len(plan.Errors) > 0 {
		job["status"] = "blocked"
		return nil
	}
	var status p.ImportReport
	if e = a.C.Request("POST", "/api/v1/runtime/import/"+id+"/begin", p.Record{"source": a.Source, "sourceId": snapshot.SourceID, "expectedRows": len(plan.Rows), "sourceRows": plan.SourceRows, "exportedAt": snapshot.ExportedAt}, &status); e != nil {
		return e
	}
	if status.Status == "committed" {
		job["status"] = "committed"
		return nil
	}
	chunk := []p.ImportRow{}
	size := 0
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		e := a.C.Request("POST", "/api/v1/runtime/import/"+id+"/stage", p.Record{"rows": chunk}, nil)
		chunk = nil
		size = 0
		return e
	}
	for _, row := range plan.Rows {
		n := len(row.Data) + len(row.ID) + 256
		if n > 2<<20 {
			return fmt.Errorf("源记录 %s 超过单记录暂存上限，尚未写入业务数据", row.ID)
		}
		if size+n > 1<<20 {
			if e = flush(); e != nil {
				return e
			}
		}
		chunk = append(chunk, row)
		size += n
	}
	if e = flush(); e != nil {
		return e
	}
	if e = a.C.Request("POST", "/api/v1/runtime/import/"+id+"/check", p.Record{}, &status); e != nil {
		return e
	}
	job["status"] = status.Status
	if len(status.Errors) > 0 {
		job["errorCount"] = len(status.Errors)
		job["report"] = p.Text(job, "report") + "\n\n目标检查阻断：\n" + strings.Join(status.Errors, "\n")
	}
	return nil
}
func reportText(plan *Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "来源：%s\n原始记录：%d\n完整归档：%d\n\n已转换：\n", plan.SourceID, plan.SourceRows, plan.ArchiveRows)
	for _, key := range sortedKeys(plan.Mapped) {
		fmt.Fprintf(&b, "%s：%d\n", key, plan.Mapped[key])
	}
	if len(plan.ArchiveOnly) > 0 {
		b.WriteString("\n仅保留原始数据的表：\n")
		for _, key := range sortedKeys(plan.ArchiveOnly) {
			fmt.Fprintf(&b, "%s：%d\n", key, plan.ArchiveOnly[key])
		}
	}
	if len(plan.Warnings) > 0 {
		b.WriteString("\n需要核对：\n" + strings.Join(plan.Warnings, "\n"))
	}
	if len(plan.Errors) > 0 {
		b.WriteString("\n阻断问题（不会提交）：\n" + strings.Join(plan.Errors, "\n"))
	}
	return b.String()
}
func (a *app) archive(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	job, e := a.Find("jobs", r.URL.Query().Get("jobId"))
	if c.Err(w, e) {
		return
	}
	if p.Text(job, "status") != "committed" {
		p.Fail(w, 400, "原始快照保留在插件状态目录，提交前请查看预览报告")
		return
	}
	offset := 0
	_, _ = fmt.Sscan(r.URL.Query().Get("offset"), &offset)
	var result p.Record
	e = a.C.Request("POST", "/api/v1/runtime/query", p.Record{"collection": "archive", "filters": []p.Record{{"field": "runSource", "value": job["sourceId"]}, {"field": "table", "value": r.URL.Query().Get("table")}}, "offset": offset, "limit": 50}, &result)
	if !c.Err(w, e) {
		p.JSON(w, result)
	}
}

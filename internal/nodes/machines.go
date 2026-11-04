package nodes

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) nextNumber(collection, key string) (float64, error) {
	rows, e := a.C.List(collection)
	value := 0.0
	for _, row := range rows {
		value = max(value, p.Number(row, key))
	}
	return value + 1, e
}

func sortServers(rows []p.Record) {
	slices.SortFunc(rows, func(a, b p.Record) int {
		na, nb := p.Number(a, "number"), p.Number(b, "number")
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		return strings.Compare(p.Text(a, "id"), p.Text(b, "id"))
	})
}

func (a *app) machinesView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可管理服务器")
	}
	rows, e := a.C.List("machines")
	if e != nil {
		return p.View{}, e
	}
	machines := p.Resource{
		Key: "machines", Title: "服务器", Endpoint: "/machines", CreateLabel: "添加服务器", Editable: true, Deletable: true,
		Columns: []p.Field{
			c.Field("number", "服务器编号", "number", false),
			c.Field("name", "名称", "text", false),
			c.Field("region", "地区", "text", false),
			c.Field("nodeCount", "关联节点", "number", false),
			c.Field("lastSeen", "最近心跳", "datetime", false),
			c.Field("enabled", "启用", "boolean", false),
		},
		Fields: []p.Field{
			c.Field("name", "名称", "text", true),
			c.Field("region", "地区", "text", false),
			{Key: "enabled", Label: "启用", Type: "boolean", Default: true},
		},
		Actions: []p.Action{
			{Key: "credentials", Label: "接入凭据"},
			{Key: "configuration", Label: "迁移面板", Fields: []p.Field{
				{Key: "panelType", Label: "目标面板", Type: "select", Required: true, Default: "pxboard", Options: []p.Option{{Label: "PxBoard", Value: "pxboard"}, {Label: "Xboard", Value: "xboard"}}},
				c.Field("api", "目标面板地址", "url", true),
				c.Field("machineId", "目标服务器编号", "number", true),
				c.Field("key", "目标服务器密钥", "password", true),
			}},
			{Key: "rotate", Label: "轮换密钥", Confirm: "轮换后原服务器密钥立即失效，需要更新 NodeRS 配置。"},
		},
	}
	configurations := p.Resource{
		Key: "machineConfigurations", Title: "配置变更", Endpoint: "/machine-configurations", PollIntervalSeconds: 10,
		Columns: []p.Field{c.Field("machineName", "原服务器", "text", false), c.Field("revision", "配置版本", "number", false), c.Field("api", "目标面板", "text", false), c.Field("machineId", "目标服务器编号", "number", false), c.Field("statusLabel", "状态", "text", false), c.Field("message", "处理结果", "text", false), c.Field("updatedAt", "更新时间", "datetime", false)},
		Actions: []p.Action{{Key: "retry", Label: "重新下发", When: map[string][]string{"status": {"failed"}}}},
	}
	return p.View{
		Title:     "服务器管理",
		Metrics:   []p.Metric{{Label: "全部服务器", Value: len(rows)}, {Label: "在线服务器", Value: onlineServerCount(rows, time.Now())}},
		Resources: []p.Resource{machines, configurations},
	}, nil
}

func (a *app) machineCredentials(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.Find("machines", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if r.PathValue("action") == "configuration" {
		a.configureMachine(w, r, row)
		return
	}
	if r.PathValue("action") == "rotate" {
		row["key"] = p.ID() + p.ID()
		if c.Err(w, a.C.Put("machines", row)) {
			return
		}
	} else if r.PathValue("action") != "credentials" {
		p.Fail(w, 404, "操作不存在")
		return
	}
	p.JSON(w, p.Record{"token": row["key"], "identifier": row["number"], "identifierLabel": "服务器编号"})
}

func (a *app) machine(w http.ResponseWriter, r *http.Request) (p.Record, bool) {
	number, e := strconv.ParseInt(r.PathValue("machine"), 10, 64)
	if e != nil {
		p.Fail(w, 400, "服务器编号无效")
		return nil, false
	}
	rows, e := a.C.List("machines")
	if c.Err(w, e) {
		return nil, false
	}
	for _, row := range rows {
		if int64(p.Number(row, "number")) == number && p.Bool(row, "enabled") && r.Header.Get("X-Machine-Key") == p.Text(row, "key") && p.Text(row, "key") != "" {
			return row, true
		}
	}
	p.Fail(w, 401, "服务器凭据无效或服务器已停用")
	return nil, false
}

func (a *app) machineNode(w http.ResponseWriter, r *http.Request, machine p.Record) (p.Record, bool) {
	number, e := strconv.ParseInt(r.PathValue("node"), 10, 64)
	if e != nil {
		p.Fail(w, 400, "节点编号无效")
		return nil, false
	}
	rows, e := a.C.List("nodes")
	if c.Err(w, e) {
		return nil, false
	}
	for _, row := range rows {
		if int64(p.Number(row, "wireId")) == number && row["machineId"] == machine["number"] && p.Bool(row, "enabled") && p.Text(row, "engine") == "noders" {
			return row, true
		}
	}
	p.Fail(w, 404, "节点不属于此服务器或已停用")
	return nil, false
}

func sanitizeServerRecord(row p.Record, nodeCount int) p.Record {
	out := p.Record{
		"id":        row["id"],
		"number":    row["number"],
		"name":      row["name"],
		"enabled":   p.Bool(row, "enabled"),
		"nodeCount": nodeCount,
		"lastSeen":  row["lastSeen"],
		"createdAt": row["createdAt"],
	}
	if region := strings.TrimSpace(p.Text(row, "region")); region != "" {
		out["region"] = region
	}
	return out
}

func (a *app) machinesCRUD() {
	a.Mux.HandleFunc("GET /machines", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		rows, e := a.C.List("machines")
		if c.Err(w, e) {
			return
		}
		nodes, e := a.C.List("nodes")
		if c.Err(w, e) {
			return
		}
		counts := map[int64]int{}
		for _, n := range primaryNodes(nodes) {
			if p.Text(n, "engine") == "noders" {
				counts[int64(p.Number(n, "machineId"))]++
			}
		}
		sortServers(rows)
		out := make([]p.Record, 0, len(rows))
		for _, row := range rows {
			out = append(out, sanitizeServerRecord(row, counts[int64(p.Number(row, "number"))]))
		}
		p.JSON(w, out)
	})
	a.Mux.HandleFunc("POST /machines", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		name := strings.TrimSpace(p.Text(b, "name"))
		if name == "" {
			p.Fail(w, 400, "请填写服务器名称")
			return
		}
		number, e := a.nextNumber("machines", "number")
		if c.Err(w, e) {
			return
		}
		enabled := true
		if raw, exists := b["enabled"]; exists {
			enabled, _ = raw.(bool)
		}
		row := p.Record{
			"name":      name,
			"region":    strings.TrimSpace(p.Text(b, "region")),
			"number":    number,
			"key":       p.ID() + p.ID(),
			"enabled":   enabled,
			"createdAt": c.Now(),
		}
		if c.Err(w, a.C.Put("machines", row)) {
			return
		}
		p.JSON(w, p.Record{
			"id":              row["id"],
			"number":          row["number"],
			"name":            row["name"],
			"token":           row["key"],
			"identifier":      row["number"],
			"identifierLabel": "服务器编号",
		})
	})
	a.Mux.HandleFunc("PATCH /machines/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		old, e := a.Find("machines", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if _, ok := b["name"]; ok {
			old["name"] = strings.TrimSpace(p.Text(b, "name"))
		}
		if _, ok := b["region"]; ok {
			old["region"] = strings.TrimSpace(p.Text(b, "region"))
		}
		if raw, ok := b["enabled"]; ok {
			old["enabled"], _ = raw.(bool)
		}
		if strings.TrimSpace(p.Text(old, "name")) == "" {
			p.Fail(w, 400, "请填写服务器名称")
			return
		}
		if c.Err(w, a.C.Put("machines", old)) {
			return
		}
		p.JSON(w, sanitizeServerRecord(old, 0))
	})
	a.Mux.HandleFunc("DELETE /machines/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		row, e := a.Find("machines", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		nodes, e := a.C.List("nodes")
		if c.Err(w, e) {
			return
		}
		for _, n := range nodes {
			if p.Text(n, "engine") == "noders" && int64(p.Number(n, "machineId")) == int64(p.Number(row, "number")) {
				p.Fail(w, 400, "请先移除或迁移此服务器下的节点")
				return
			}
		}
		if !c.Err(w, a.C.Delete("machines", r.PathValue("id"))) {
			p.JSON(w, true)
		}
	})
}

func (a *app) machineAPI() {
	a.machinesCRUD()
	a.View("machines", a.machinesView)
	a.Mux.HandleFunc("POST /machines/{id}/{action}", a.machineCredentials)
	base := "/machines/{machine}"
	a.Mux.HandleFunc("GET "+base+"/handshake", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.machine(w, r); ok {
			p.JSON(w, p.Record{"apiVersion": "1.0", "transport": "poll", "capabilities": []string{"multiNode", "trafficOutbox", "onlineAccounts", "managedConfiguration"}})
		}
	})
	a.configurationAPI(base)
	a.Mux.HandleFunc("GET "+base+"/nodes", a.machineNodes)
	a.Mux.HandleFunc("GET "+base+"/nodes/{node}/config", a.machineConfig)
	a.Mux.HandleFunc("GET "+base+"/nodes/{node}/users", a.machineUsers)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/traffic", a.machineTraffic)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/alive", a.machineTelemetry)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/status", a.machineTelemetry)
	a.Mux.HandleFunc("POST "+base+"/status", a.machineTelemetry)
}

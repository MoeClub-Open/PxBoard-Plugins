package subscriptions

import (
	"fmt"
	"math"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) templateAPI() {
	a.Mux.HandleFunc("GET /templates", a.templates)
	a.Mux.HandleFunc("POST /templates", a.saveTemplate)
	a.Mux.HandleFunc("PATCH /templates/{id}", a.saveTemplate)
	a.Mux.HandleFunc("DELETE /templates/{id}", a.deleteTemplate)
	a.Mux.HandleFunc("GET /templates/options", a.templateOptions)
	a.Mux.HandleFunc("POST /templates/preview", a.previewTemplate)
	a.Mux.HandleFunc("POST /templates/import-xboard", a.importTemplates)
	a.Mux.HandleFunc("POST /templates/{id}/{action}", a.templateAction)
	a.Mux.HandleFunc("GET /template-settings", a.templatePolicy)
	a.Mux.HandleFunc("PATCH /template-settings", a.templatePolicy)
}

func (a *app) templates(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	settings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	rows, err := a.C.List("templates")
	if c.Err(w, err) {
		return
	}
	for _, row := range rows {
		row["isDefault"] = p.Text(settings, templateKey(p.Text(row, "format"))) == p.Text(row, "id")
	}
	p.JSON(w, rows)
}

func (a *app) saveTemplate(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	settings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	var body p.Record
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	row := p.Record{"id": p.ID(), "format": "clash", "enabled": true, "createdAt": c.Now()}
	if r.Method == http.MethodPatch {
		row, err = a.C.Get("templates", r.PathValue("id"))
		if c.Err(w, err) {
			return
		}
	}
	for _, key := range []string{"name", "format", "content", "enabled"} {
		if value, exists := body[key]; exists {
			row[key] = value
		}
	}
	row["name"] = strings.TrimSpace(p.Text(row, "name"))
	if !c.Allowed(p.Text(row, "format"), "clash", "sing-box") || p.Text(row, "name") == "" {
		p.Fail(w, 400, "请填写模板名称并选择格式")
		return
	}
	if strings.TrimSpace(p.Text(row, "content")) == "" {
		row["content"] = sc.Default(p.Text(row, "format"))
	}
	for _, format := range []string{"clash", "sing-box"} {
		if p.Text(settings, templateKey(format)) == p.Text(row, "id") && (!p.Bool(row, "enabled") || p.Text(row, "format") != format) {
			p.Fail(w, 400, "请先切换默认模板，再停用或更改其格式")
			return
		}
	}
	validationErr := sc.Check(p.Text(row, "format"), p.Text(row, "content"))
	if validationErr != nil && p.Bool(row, "enabled") {
		c.Err(w, validationErr)
		return
	}
	row["updatedAt"] = c.Now()
	delete(row, "validationError")
	if validationErr != nil {
		row["validationError"] = validationErr.Error()
	}
	if !c.Err(w, a.C.Put("templates", row)) {
		p.JSON(w, row)
	}
}

func (a *app) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	id := r.PathValue("id")
	if id == p.Text(config, "clashTemplateId") || id == p.Text(config, "singboxTemplateId") {
		p.Fail(w, 400, "请先切换默认模板再删除")
		return
	}
	if !c.Err(w, a.C.Delete("templates", id)) {
		p.JSON(w, true)
	}
}

func (a *app) templateOptions(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	if _, err := a.templateSettings(); c.Err(w, err) {
		return
	}
	rows, err := a.C.List("templates")
	if c.Err(w, err) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		if p.Bool(row, "enabled") && p.Text(row, "format") == r.URL.Query().Get("format") {
			out = append(out, p.Record{"id": row["id"], "name": row["name"]})
		}
	}
	p.JSON(w, out)
}

func (a *app) templateAction(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, err := a.C.Get("templates", r.PathValue("id"))
	if c.Err(w, err) {
		return
	}
	format, content := p.Text(row, "format"), p.Text(row, "content")
	switch r.PathValue("action") {
	case "validate":
		if !c.Err(w, sc.Check(format, content)) {
			p.JSON(w, p.Record{"message": "模板语法、筛选和引用校验通过"})
		}
	case "preview":
		a.sendPreview(w, format, content)
	case "default":
		if !p.Bool(row, "enabled") {
			p.Fail(w, 400, "请先启用模板")
			return
		}
		if c.Err(w, sc.Check(format, content)) {
			return
		}
		settings, err := a.templateSettings()
		if c.Err(w, err) {
			return
		}
		settings[templateKey(format)] = row["id"]
		if !c.Err(w, a.C.Put("settings", settings)) {
			p.JSON(w, p.Record{"message": "已设为默认模板"})
		}
	case "reset":
		row["content"] = sc.Default(format)
		delete(row, "validationError")
		row["updatedAt"] = c.Now()
		if !c.Err(w, a.C.Put("templates", row)) {
			p.JSON(w, p.Record{"message": "已恢复内置模板内容"})
		}
	default:
		p.Fail(w, 404, "操作不存在")
	}
}

func (a *app) previewTemplate(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var body struct{ Format, Content string }
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	a.sendPreview(w, body.Format, body.Content)
}
func (a *app) sendPreview(w http.ResponseWriter, format, content string) {
	settings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	data, err := sc.Render(format, content, sc.PreviewNodes(format), a.profileTitle(settings), []string{"subscription.example.com"})
	if !c.Err(w, err) {
		p.JSON(w, p.Record{"title": "订阅预览（示例节点）", "details": string(data)})
	}
}

func (a *app) templatePolicy(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	settings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	if r.Method == http.MethodGet {
		p.JSON(w, []p.Record{settings})
		return
	}
	var body p.Record
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	for _, key := range []string{"clashTemplateId", "singboxTemplateId", "profileTitle", "homepage", "updateIntervalHours"} {
		if value, exists := body[key]; exists {
			settings[key] = value
		}
	}
	interval := p.Number(settings, "updateIntervalHours")
	if interval < 1 || interval > 168 || math.Trunc(interval) != interval || !validHomepage(p.Text(settings, "homepage")) {
		p.Fail(w, 400, "刷新周期须为 1–168 小时，面板地址须为有效的 HTTP/HTTPS 地址")
		return
	}
	for _, format := range []string{"clash", "sing-box"} {
		row, err := a.C.Get("templates", p.Text(settings, templateKey(format)))
		if err != nil || !p.Bool(row, "enabled") || p.Text(row, "format") != format {
			p.Fail(w, 400, fmt.Sprintf("%s 默认模板不存在或未启用", format))
			return
		}
	}
	if !c.Err(w, a.C.Put("settings", settings)) {
		p.JSON(w, settings)
	}
}

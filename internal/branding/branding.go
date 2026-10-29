package branding

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /public", a.public)
	a.Mux.HandleFunc("GET /_site", a.site)
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.View("settings", a.view)
	return a
}

func (a *app) config() (p.Record, error) {
	rows, e := a.C.List("settings")
	if e != nil {
		return nil, e
	}
	out := p.Record{"id": "settings", "name": "", "logoUrl": "", "faviconUrl": "", "seoTitle": "", "description": "", "heading": "", "introduction": "", "highlights": "", "footer": "", "socialImageUrl": "", "indexingEnabled": true}
	for _, row := range rows {
		if p.Text(row, "id") != "settings" {
			continue
		}
		for field := range out {
			if value, ok := row[field]; ok {
				out[field] = value
			}
		}
		break
	}
	return out, nil
}

func (a *app) public(w http.ResponseWriter, r *http.Request) {
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, p.Record{"name": config["name"], "logoUrl": config["logoUrl"], "faviconUrl": config["faviconUrl"]})
}

func imageURL(value string) bool {
	if value == "" {
		return true
	}
	u, e := url.Parse(value)
	if e != nil || strings.ContainsAny(value, "\\\r\n") {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return true
	}
	return u.Scheme == "https" && u.Host != "" && u.User == nil
}

func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, []p.Record{config})
		return
	}
	var body p.Record
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	for _, field := range []string{"name", "logoUrl", "faviconUrl", "seoTitle", "description", "heading", "introduction", "highlights", "footer", "socialImageUrl"} {
		if value, exists := body[field]; exists {
			text, ok := value.(string)
			if !ok {
				p.Fail(w, 400, "品牌信息格式无效")
				return
			}
			config[field] = strings.TrimSpace(text)
		}
	}
	if value, ok := body["indexingEnabled"]; ok {
		enabled, valid := value.(bool)
		if !valid {
			p.Fail(w, 400, "搜索收录开关格式无效")
			return
		}
		config["indexingEnabled"] = enabled
	}
	if !imageURL(p.Text(config, "logoUrl")) || !imageURL(p.Text(config, "faviconUrl")) || !imageURL(p.Text(config, "socialImageUrl")) {
		p.Fail(w, 400, "图片地址需为 HTTPS 地址或本站绝对路径")
		return
	}
	config["updatedAt"] = c.Now()
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, true)
	}
}

func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权修改品牌设置")
	}
	fields := []p.Field{
		c.Field("name", "站点名称", "text", false),
		c.Field("logoUrl", "Logo 地址", "text", false),
		c.Field("faviconUrl", "网站图标地址", "text", false),
	}
	home := []p.Field{c.Field("heading", "首页标题", "text", false), c.Field("introduction", "服务介绍", "textarea", false), c.Field("highlights", "服务要点（每行一条）", "textarea", false), c.Field("footer", "页脚信息", "textarea", false), c.Field("seoTitle", "搜索与分享标题", "text", false), c.Field("description", "搜索与分享摘要", "textarea", false), c.Field("socialImageUrl", "分享图片地址", "url", false), {Key: "indexingEnabled", Label: "允许搜索引擎收录商业首页", Type: "boolean", Default: true}}
	return p.View{Title: "站点设置", Resources: []p.Resource{{Key: "settings", Title: "品牌信息", Endpoint: "/settings", Singleton: true, Editable: true, Fields: fields, Columns: fields}, {Key: "homepage", Title: "首页与搜索", Endpoint: "/settings", Singleton: true, Editable: true, Fields: home, Columns: home}}}, nil
}

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
	out := p.Record{"id": "settings", "name": "", "logoUrl": "", "faviconUrl": ""}
	for _, row := range rows {
		if p.Text(row, "id") != "settings" {
			continue
		}
		for _, field := range []string{"name", "logoUrl", "faviconUrl"} {
			out[field] = p.Text(row, field)
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
	for _, field := range []string{"name", "logoUrl", "faviconUrl"} {
		if value, exists := body[field]; exists {
			text, ok := value.(string)
			if !ok {
				p.Fail(w, 400, "品牌信息格式无效")
				return
			}
			config[field] = strings.TrimSpace(text)
		}
	}
	if !imageURL(p.Text(config, "logoUrl")) || !imageURL(p.Text(config, "faviconUrl")) {
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
	return p.View{Title: "品牌设置", Resources: []p.Resource{{Key: "settings", Title: "品牌信息", Endpoint: "/settings", Singleton: true, Editable: true, Fields: fields, Columns: fields}}}, nil
}

package branding

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const defaultIntroduction = "提供加密网络接入，按访问需求选择线路，并在账户中集中管理订阅。"

type routeEntry struct {
	Title       string `json:"title"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

type featureEntry struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func defaultFeatures() []featureEntry {
	return []featureEntry{
		{
			Title:       "导入订阅即可连接",
			Description: "将订阅地址导入兼容客户端，在电脑、手机和路由器上使用。",
		},
		{
			Title:       "按设备管理订阅",
			Description: "为不同设备创建独立链接，按需绑定客户端或重置链接。",
		},
		{
			Title:       "套餐与用量随时查看",
			Description: "在账户中查看流量用量、套餐有效期与订阅信息。",
		},
	}
}

func parseRoutes(raw any) ([]routeEntry, bool) {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]routeEntry); ok {
			return typed, true
		}
		return nil, false
	}
	out := make([]routeEntry, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		entry := routeEntry{
			Title:       strings.TrimSpace(p.Text(row, "title")),
			Region:      strings.TrimSpace(p.Text(row, "region")),
			Description: strings.TrimSpace(p.Text(row, "description")),
		}
		if entry.Title == "" && entry.Region == "" && entry.Description == "" {
			continue
		}
		if entry.Title == "" {
			entry.Title = entry.Region
		}
		out = append(out, entry)
	}
	return out, true
}

func parseFeatures(raw any) ([]featureEntry, bool) {
	items, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]featureEntry); ok {
			return typed, true
		}
		return nil, false
	}
	out := make([]featureEntry, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		entry := featureEntry{
			Title:       strings.TrimSpace(p.Text(row, "title")),
			Description: strings.TrimSpace(p.Text(row, "description")),
		}
		if entry.Title == "" && entry.Description == "" {
			continue
		}
		if entry.Title == "" {
			entry.Title = entry.Description
			entry.Description = ""
		}
		out = append(out, entry)
	}
	return out, true
}

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
	out := p.Record{
		"id":              "settings",
		"name":            "",
		"logoUrl":         "",
		"faviconUrl":      "",
		"seoTitle":        "",
		"description":     "",
		"heading":         "",
		"introduction":    defaultIntroduction,
		"routes":          []routeEntry{},
		"features":        defaultFeatures(),
		"footer":          "",
		"socialImageUrl":  "",
		"indexingEnabled": true,
	}
	for _, row := range rows {
		if p.Text(row, "id") != "settings" {
			continue
		}
		for _, field := range []string{"name", "logoUrl", "faviconUrl", "seoTitle", "description", "heading", "footer", "socialImageUrl"} {
			if value, ok := row[field]; ok {
				out[field] = value
			}
		}
		if value, ok := row["introduction"]; ok && strings.TrimSpace(p.Text(row, "introduction")) != "" {
			out["introduction"] = value
		} else if p.Bool(row, "introductionCustomized") {
			out["introduction"] = p.Text(row, "introduction")
		}
		if p.Bool(row, "introductionCustomized") {
			out["introductionCustomized"] = true
		}
		if value, ok := row["indexingEnabled"]; ok {
			out["indexingEnabled"] = value
		}
		if rawRoutes, ok := row["routes"]; ok {
			if parsed, valid := parseRoutes(rawRoutes); valid {
				out["routes"] = parsed
			}
		}
		if rawFeatures, ok := row["features"]; ok {
			if parsed, valid := parseFeatures(rawFeatures); valid {
				out["features"] = parsed
			}
		} else if legacy := strings.TrimSpace(p.Text(row, "highlights")); legacy != "" {
			converted := []featureEntry{}
			for _, line := range strings.Split(legacy, "\n") {
				if text := strings.TrimSpace(line); text != "" {
					converted = append(converted, featureEntry{Title: text})
				}
			}
			if len(converted) > 0 {
				out["features"] = converted
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
	for _, field := range []string{"name", "logoUrl", "faviconUrl", "seoTitle", "description", "heading", "introduction", "footer", "socialImageUrl"} {
		if value, exists := body[field]; exists {
			text, ok := value.(string)
			if !ok {
				p.Fail(w, 400, "品牌信息格式无效")
				return
			}
			config[field] = strings.TrimSpace(text)
			if field == "introduction" {
				config["introductionCustomized"] = true
			}
		}
	}
	if rawRoutes, exists := body["routes"]; exists {
		routes, ok := parseRoutes(rawRoutes)
		if !ok {
			p.Fail(w, 400, "线路介绍格式无效")
			return
		}
		config["routes"] = routes
	}
	if rawFeatures, exists := body["features"]; exists {
		features, ok := parseFeatures(rawFeatures)
		if !ok {
			p.Fail(w, 400, "功能特性格式无效")
			return
		}
		config["features"] = features
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
	home := []p.Field{
		c.Field("heading", "首页标题", "text", false),
		c.Field("introduction", "品牌介绍", "textarea", false),
		{Key: "routes", Label: "线路介绍", Type: "siteRoutes"},
		{Key: "features", Label: "功能特性", Type: "siteFeatures"},
		c.Field("footer", "页脚信息", "textarea", false),
		c.Field("seoTitle", "搜索与分享标题", "text", false),
		c.Field("description", "搜索与分享摘要", "textarea", false),
		c.Field("socialImageUrl", "分享图片地址", "url", false),
		{Key: "indexingEnabled", Label: "允许搜索引擎收录商业首页", Type: "boolean", Default: true},
	}
	return p.View{
		Title: "站点设置",
		Resources: []p.Resource{
			{Key: "settings", Title: "品牌信息", Endpoint: "/settings", Singleton: true, Editable: true, Fields: fields, Columns: fields},
			{Key: "homepage", Title: "首页与搜索", Endpoint: "/settings", Singleton: true, Editable: true, Fields: home, Columns: home},
		},
	}, nil
}

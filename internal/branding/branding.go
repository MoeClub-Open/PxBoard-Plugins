package branding

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const defaultIntroduction = "覆盖亚太、北美与欧洲机房的加密网络接入服务，为跨区域访问、远程协作与日常终端连接提供稳定通道。"

type routeEntry struct {
	Title       string `json:"title"`
	Region      string `json:"region"`
	Description string `json:"description"`
}

type featureEntry struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func defaultRoutes() []routeEntry {
	return []routeEntry{
		{
			Title:       "亚太高速接入",
			Region:      "香港 · 日本 · 新加坡",
			Description: "毗邻核心国际海缆与优质直连入口，提供低延迟响应，适合高频网页浏览、即时通讯与流媒体播放。",
		},
		{
			Title:       "北美骨干互联",
			Region:      "美西 · 美东",
			Description: "接入高带宽北美数据中心骨干网络，兼顾跨洋吞吐能力，适合开发工具、云端平台与大体积数据传输。",
		},
		{
			Title:       "欧洲多点覆盖",
			Region:      "德国 · 荷兰 · 瑞士",
			Description: "部署多国独立接入节点与冗余出口路由，满足跨区域业务访问、地区内容解锁及备用线路切换。",
		},
		{
			Title:       "抗干扰传输架构",
			Region:      "TLS · IPv4 / IPv6",
			Description: "采用现代加密传输协议与多栈接入地址，针对晚高峰与复杂网络环境优化连接稳定性。",
		},
	}
}

func defaultFeatures() []featureEntry {
	return []featureEntry{
		{
			Title:       "主流客户端通用订阅",
			Description: "通过同一订阅地址按客户端标识自动下发节点与分流规则，支持桌面端、移动端及路由设备直接导入。",
		},
		{
			Title:       "多链接与凭据隔离",
			Description: "支持按设备单独创建订阅链接并绑定客户端类型，连接凭据与账户密码相互独立，异常时可随时重置。",
		},
		{
			Title:       "自助管理与状态可视",
			Description: "在线完成多周期开通、套餐折抵升级、兑换码核销与工单咨询，实时查看周期流量明细与服务器负载。",
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
		"routes":          defaultRoutes(),
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
		} else if _, customIntro := row["introductionCustomized"]; customIntro {
			out["introduction"] = p.Text(row, "introduction")
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

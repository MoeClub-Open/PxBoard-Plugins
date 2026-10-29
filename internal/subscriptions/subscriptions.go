package subscriptions

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{App: c.New()}
	a.templateAPI()
	a.View("subscriptions", a.view)
	a.Mux.HandleFunc("GET /links", a.links)
	a.Mux.HandleFunc("POST /links", a.create)
	a.Mux.HandleFunc("POST /links/{id}/rotate", a.rotate)
	a.Mux.HandleFunc("DELETE /links/{id}", a.deleteLink)
	a.Mux.HandleFunc("POST /links/{id}/verify", a.verifyLink)
	a.Mux.HandleFunc("GET /verification", a.verificationSettings)
	a.Mux.HandleFunc("PATCH /verification", a.verificationSettings)
	a.Mux.HandleFunc("GET /verification/settings", a.verificationSettings)
	a.Mux.HandleFunc("PATCH /verification/settings", a.verificationSettings)
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /routing", a.routing)
	a.Mux.HandleFunc("PATCH /routing", a.routing)
	a.Mux.HandleFunc("GET /settings/public", a.publicSettings)
	a.Mux.HandleFunc("GET /s/{token}", a.export)
	a.Mux.HandleFunc("GET /configuration", a.configuration)
	return a
}
func (a *app) view(r *http.Request) (p.View, error) {
	res := p.Resource{Paginated: true, Key: "links", Title: "订阅链接", Endpoint: "/links", CreateLabel: "创建订阅", Empty: "开通访问账户后，创建订阅链接", Deletable: true, Columns: []p.Field{c.Field("email", "用户", "text", false), c.Field("name", "名称", "text", false), c.Field("boundClient", "绑定客户端", "text", false), c.Field("shareUrl", "通用订阅", "link", false)}, Actions: []p.Action{{Key: "rotate", Label: "重置链接", Confirm: "旧订阅链接会立即失效，并解除客户端绑定，确定重置？"}}}
	if p.Admin(r) {
		res.Fields = []p.Field{c.UserField(), c.Field("name", "名称", "text", false), {Key: "bindUserAgent", Label: "绑定首次使用的客户端", Type: "boolean", Hint: "客户端标识可以伪造，不能代替保管链接"}}
		setting := p.Field{Key: "enabled", Label: "订阅拉取前要求人机验证", Type: "boolean", Hint: "启用后，用户需在订阅页面验证一次，客户端才能继续自动更新；关闭后重新启用时需重新验证。"}
		browser := p.Field{Key: "hideFromBrowsers", Label: "浏览器访问时隐藏订阅", Type: "boolean", Hint: "按请求的 User-Agent 判断，可被伪造"}
		agents := p.Field{Key: "browserAgents", Label: "浏览器 User-Agent 关键词", Type: "textarea", Hint: "每行一个关键词，不区分大小写；匹配时返回伪装页面"}
		routeFields := []p.Field{{Key: "enabled", Label: "使用自定义订阅地址", Type: "boolean"}, {Key: "baseUrls", Label: "订阅域名", Type: "textarea", Hint: "多个地址用逗号或换行分隔，留空使用面板域名；域名需指向面板"}, {Key: "path", Label: "订阅路径", Type: "text"}}
		return p.View{Title: "订阅与客户端", Resources: []p.Resource{res, templateResource(), templateSettingsResource(), {Key: "settings", Title: "订阅访问", Endpoint: "/settings", Singleton: true, Editable: true, Fields: []p.Field{browser, agents}, Columns: []p.Field{browser, agents}}, {Key: "routing", Title: "订阅地址", Endpoint: "/routing", Singleton: true, Editable: true, Fields: routeFields, Columns: routeFields}, {Key: "verification", Title: "订阅验证", Endpoint: "/verification/settings", Singleton: true, Editable: true, Fields: []p.Field{setting}, Columns: []p.Field{setting}}}}, nil
	}
	return p.View{Title: "订阅与客户端", Notice: "Mieru 节点请使用 Mihomo；sing-box 订阅仅包含其支持的节点。", Resources: []p.Resource{res}}, nil
}

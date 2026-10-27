package subscriptions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{App: c.New()}
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
	a.Mux.HandleFunc("GET /settings/public", a.publicSettings)
	a.Mux.HandleFunc("GET /s/{token}", a.export)
	return a
}
func (a *app) export(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	settings, e := a.subscriptionSettings()
	if c.Err(w, e) {
		return
	}
	if p.Bool(settings, "hideFromBrowsers") && matchesBrowser(r, p.Text(settings, "browserAgents")) {
		disguise(w)
		return
	}
	rows, e := a.queryLinks([]p.Record{{"field": "token", "value": r.PathValue("token")}})
	if c.Err(w, e) {
		return
	}
	if len(rows) == 0 {
		p.Fail(w, 404, "订阅链接不存在或已重置")
		return
	}
	link := rows[0]
	client, major, ok := a.checkClient(w, r, link, p.Text(settings, "browserAgents"))
	if !ok {
		return
	}
	policy, e := a.verificationPolicy()
	if c.Err(w, e) {
		return
	}
	if p.Bool(policy, "enabled") && !verified(link, policy) {
		p.Fail(w, 403, "请先登录面板并完成订阅验证")
		return
	}
	var clients, nodes []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients?userId="+url.QueryEscape(p.Text(link, "userId")), nil, &clients)) {
		return
	}
	var account p.Record
	for _, v := range clients {
		if v["userId"] == link["userId"] {
			account = v
		}
	}
	if account == nil {
		p.Fail(w, 403, "账户已到期、暂停或流量不足")
		return
	}
	if c.Err(w, a.C.Call("nodes", "GET", "/internal/nodes", nil, &nodes)) {
		return
	}
	allowed := []p.Record{}
	for _, n := range nodes {
		parent, hasParent := n["parentAccess"].(map[string]any)
		if c.CanUse(account, n) && (!hasParent || c.CanUse(account, p.Record(parent))) {
			allowed = append(allowed, n)
		}
	}
	w.Header().Set("profile-update-interval", "6")
	info := fmt.Sprintf("upload=0; download=%.0f; total=%.0f", p.Number(account, "usedGiB")*1073741824, p.Number(account, "quotaGiB")*1073741824)
	if expires, e := time.Parse(time.RFC3339, p.Text(account, "expiresAt")); e == nil {
		info += fmt.Sprintf("; expire=%d", expires.Unix())
	}
	w.Header().Set("Subscription-Userinfo", info)
	format := subscriptionFormat(r)
	if !c.Allowed(format, "clash", "sing-box", "uri") {
		p.Fail(w, 400, "支持 clash、sing-box、uri 格式")
		return
	}
	compatible := []p.Record{}
	skipped := map[string]bool{}
	for _, n := range allowed {
		protocol := p.Text(n, "protocol")
		if supportedFormat(n, format) {
			compatible = append(compatible, n)
		} else {
			skipped[protocol] = true
		}
	}
	if len(skipped) > 0 {
		protocols := []string{}
		for protocol := range skipped {
			protocols = append(protocols, protocol)
		}
		sort.Strings(protocols)
		w.Header().Set("X-PxBoard-Skipped-Protocols", strings.Join(protocols, ","))
		if len(compatible) == 0 {
			message := "当前格式没有可用节点（协议或传输不支持：" + strings.Join(protocols, "、") + "）"
			if skipped["mieru"] {
				message += "；Mieru 请使用 Mihomo 订阅或支持 mierus 链接的客户端"
			}
			p.Fail(w, 400, message)
			return
		}
	}
	allowed = sortedNodes(compatible)
	uuid := p.Text(account, "credential")
	var data []byte
	var err error
	var contentType string
	switch format {
	case "clash":
		proxies := []any{}
		names := []string{}
		for _, n := range allowed {
			proxy := clash(n, subscriptionPassword(account, n), uuid)
			proxies = append(proxies, proxy)
			names = append(names, p.Text(n, "name"))
		}
		if len(names) == 0 {
			names = []string{"DIRECT"}
		}
		config := map[string]any{"proxies": proxies, "proxy-groups": []any{map[string]any{"name": "代理选择", "type": "select", "proxies": names}}, "rules": []string{"MATCH,代理选择"}}
		data, err = yaml.Marshal(config)
		contentType = "text/yaml; charset=utf-8"
	case "sing-box":
		out := []any{}
		tags := []string{}
		for _, n := range allowed {
			out = append(out, singbox(n, subscriptionPassword(account, n), uuid))
			tags = append(tags, p.Text(n, "name"))
		}
		if len(tags) == 0 {
			tags = []string{"direct"}
		}
		out = append([]any{map[string]any{"type": "selector", "tag": "proxy", "outbounds": tags}}, out...)
		out = append(out, map[string]any{"type": "direct", "tag": "direct"})
		data, err = json.Marshal(map[string]any{"outbounds": out, "route": map[string]any{"final": "proxy"}})
		contentType = "application/json"
	case "uri":
		lines := []string{}
		for _, n := range allowed {
			lines = append(lines, uri(n, subscriptionPassword(account, n), uuid))
		}
		data = []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n"))))
		contentType = "text/plain; charset=utf-8"
	}
	if err != nil {
		p.Fail(w, 500, "订阅生成失败")
		return
	}
	if !a.bindClient(w, link, client, major) {
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
func (a *app) view(r *http.Request) (p.View, error) {
	res := p.Resource{Key: "links", Title: "订阅链接", Endpoint: "/links", CreateLabel: "创建订阅", Empty: "开通访问账户后，创建订阅链接", Deletable: true, Columns: []p.Field{c.Field("email", "用户", "text", false), c.Field("name", "名称", "text", false), c.Field("boundClient", "绑定客户端", "text", false), c.Field("shareUrl", "通用订阅", "link", false)}, Actions: []p.Action{{Key: "rotate", Label: "重置链接", Confirm: "旧订阅链接会立即失效，并解除客户端绑定，确定重置？"}}}
	if p.Admin(r) {
		res.Fields = []p.Field{c.UserField(a.C), c.Field("name", "名称", "text", false), {Key: "bindUserAgent", Label: "绑定首次使用的客户端", Type: "boolean", Hint: "客户端标识可以伪造，不能代替保管链接"}}
		setting := p.Field{Key: "enabled", Label: "订阅拉取前要求人机验证", Type: "boolean", Hint: "启用后，用户需在订阅页面验证一次，客户端才能继续自动更新；关闭后重新启用时需重新验证。"}
		browser := p.Field{Key: "hideFromBrowsers", Label: "浏览器访问时隐藏订阅", Type: "boolean", Hint: "按请求的 User-Agent 判断，可被伪造"}
		agents := p.Field{Key: "browserAgents", Label: "浏览器 User-Agent 关键词", Type: "textarea", Hint: "每行一个关键词，不区分大小写；匹配时返回伪装页面"}
		return p.View{Title: "订阅与客户端", Resources: []p.Resource{res, {Key: "settings", Title: "订阅访问", Endpoint: "/settings", Singleton: true, Editable: true, Fields: []p.Field{browser, agents}, Columns: []p.Field{browser, agents}}, {Key: "verification", Title: "订阅验证", Endpoint: "/verification/settings", Singleton: true, Editable: true, Fields: []p.Field{setting}, Columns: []p.Field{setting}}}}, nil
	}
	return p.View{Title: "订阅与客户端", Notice: "Mieru 节点请使用 Mihomo；sing-box 订阅仅包含其支持的节点。", Resources: []p.Resource{res}}, nil
}

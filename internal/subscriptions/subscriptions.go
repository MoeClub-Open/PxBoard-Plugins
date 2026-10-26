package subscriptions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"strings"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.View("subscriptions", a.view)
	a.Mux.HandleFunc("GET /links", a.links)
	a.Mux.HandleFunc("POST /links", a.create)
	a.Mux.HandleFunc("POST /links/{id}/rotate", a.rotate)
	a.Mux.HandleFunc("GET /s/{token}", a.export)
	return a
}
func (a *app) create(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	user := p.Subject(r).ID
	if p.Admin(r) {
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if p.Text(b, "userId") != "" {
			user = p.Text(b, "userId")
		}
	}
	var clients []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients", nil, &clients)) {
		return
	}
	found := false
	for _, v := range clients {
		if v["userId"] == user {
			found = true
		}
	}
	if !found {
		p.Fail(w, 400, "用户还没有可用访问账户")
		return
	}
	rows, e := a.C.List("links")
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		if row["userId"] == user {
			p.Fail(w, 409, "此用户已经有订阅链接")
			return
		}
	}
	b := p.Record{"userId": user, "token": p.ID() + p.ID(), "createdAt": c.Now()}
	if !c.Err(w, a.C.Put("links", b)) {
		p.JSON(w, b)
	}
}
func (a *app) links(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("links", func(row p.Record) bool { return c.Own(r, row) })
	if c.Err(w, e) {
		return
	}
	users, e := a.C.Users()
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		for _, u := range users {
			if row["userId"] == u["id"] {
				row["email"] = u["email"]
			}
		}
		base := "/api/v1/extensions/subscriptions/s/" + p.Text(row, "token")
		row["clashUrl"] = base + "?format=clash"
		row["singboxUrl"] = base + "?format=sing-box"
		row["shareUrl"] = base + "?format=uri"
	}
	p.JSON(w, rows)
}
func (a *app) rotate(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.Find("links", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if !c.Own(r, row) {
		p.Fail(w, 403, "无权修改此链接")
		return
	}
	row["token"] = p.ID() + p.ID()
	if !c.Err(w, a.C.Put("links", row)) {
		p.JSON(w, true)
	}
}
func (a *app) export(w http.ResponseWriter, r *http.Request) {
	rows, e := a.C.List("links")
	if c.Err(w, e) {
		return
	}
	var link p.Record
	for _, v := range rows {
		if v["token"] == r.PathValue("token") {
			link = v
		}
	}
	if link == nil {
		p.Fail(w, 404, "订阅链接不存在或已重置")
		return
	}
	var clients, nodes []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients", nil, &clients)) {
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
		if c.CanUse(account, n) {
			allowed = append(allowed, n)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("profile-update-interval", "6")
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=%.0f; total=%.0f", p.Number(account, "usedGiB")*1073741824, p.Number(account, "quotaGiB")*1073741824))
	format := r.URL.Query().Get("format")
	if format == "" {
		ua := strings.ToLower(r.UserAgent())
		if strings.Contains(ua, "clash") {
			format = "clash"
		} else if strings.Contains(ua, "sing-box") {
			format = "sing-box"
		} else {
			format = "uri"
		}
	}
	for _, n := range allowed {
		if !c.Allowed(p.Text(n, "protocol"), "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "anytls", "tuic") {
			p.Fail(w, 400, "当前订阅格式尚未支持节点协议："+p.Text(n, "protocol"))
			return
		}
	}
	uuid := p.Text(account, "credential")
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
		rawJSON(w, map[string]any{"proxies": proxies, "proxy-groups": []any{map[string]any{"name": "代理选择", "type": "select", "proxies": names}}, "rules": []string{"MATCH,代理选择"}})
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
		rawJSON(w, map[string]any{"outbounds": out, "route": map[string]any{"final": "proxy"}})
	case "uri":
		lines := []string{}
		for _, n := range allowed {
			lines = append(lines, uri(n, subscriptionPassword(account, n), uuid))
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))))
	default:
		p.Fail(w, 400, "支持 clash、sing-box、uri 格式")
	}
}
func rawJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (a *app) view(r *http.Request) (p.View, error) {
	res := p.Resource{Key: "links", Title: "订阅链接", Endpoint: "/links", CreateLabel: "生成订阅", Empty: "开通访问账户后，生成自己的订阅链接", Columns: []p.Field{c.Field("email", "用户", "text", false), c.Field("clashUrl", "Clash / Mihomo", "link", false), c.Field("singboxUrl", "sing-box", "link", false), c.Field("shareUrl", "通用订阅", "link", false)}, Actions: []p.Action{{Key: "rotate", Label: "重置链接", Confirm: "旧订阅链接会立即失效，确定重置？"}}}
	if p.Admin(r) {
		res.Fields = []p.Field{c.UserField(a.C)}
	}
	return p.View{Title: "订阅与客户端", Resources: []p.Resource{res}}, nil
}

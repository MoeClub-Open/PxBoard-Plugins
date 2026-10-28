package subscriptions

import (
	"errors"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (a *app) queryLinks(filters []p.Record) ([]p.Record, error) {
	return a.QueryAll("links", filters...)
}

func (a *app) create(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	user := p.Subject(r).ID
	var b p.Record
	if err := p.Decode(r, &b); err != nil && !errors.Is(err, io.EOF) {
		p.Fail(w, 400, "请求格式无效")
		return
	}
	if p.Admin(r) {
		if p.Text(b, "userId") != "" {
			user = p.Text(b, "userId")
		}
	}
	var clients []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients?userId="+url.QueryEscape(user), nil, &clients)) {
		return
	}
	if len(clients) == 0 {
		p.Fail(w, 400, "用户还没有可用访问账户")
		return
	}
	bind := false
	if value, exists := b["bindUserAgent"]; exists {
		var ok bool
		bind, ok = value.(bool)
		if !ok {
			p.Fail(w, 400, "客户端绑定设置无效")
			return
		}
	}
	name := strings.TrimSpace(p.Text(b, "name"))
	if name == "" {
		name = "订阅链接"
	}
	row := p.Record{"userId": user, "name": name, "bindUserAgent": bind, "token": p.ID() + p.ID(), "createdAt": c.Now()}
	if !c.Err(w, a.C.Put("links", row)) {
		p.JSON(w, p.Record{"id": row["id"]})
	}
}

func (a *app) links(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	filters := []p.Record{}
	if !p.Admin(r) || r.URL.Query().Get("mine") == "1" {
		filters = append(filters, p.Record{"field": "userId", "value": p.Subject(r).ID})
	}
	rows, e := a.queryLinks(filters)
	if c.Err(w, e) {
		return
	}
	policy, e := a.verificationPolicy()
	if c.Err(w, e) {
		return
	}
	users, e := a.C.Users()
	if c.Err(w, e) {
		return
	}
	routes, e := a.C.List("public_routes")
	if c.Err(w, e) {
		return
	}
	customBase := ""
	for _, route := range routes {
		if p.Text(route, "id") != "primary" || !p.Bool(route, "enabled") {
			continue
		}
		paths, err := p.PublicRoutePaths(route)
		if err != nil {
			continue
		}
		values := strings.FieldsFunc(p.Text(route, "baseUrls"), func(r rune) bool { return r == ',' || r == '\n' || r == '\r' })
		if len(values) == 0 {
			customBase = paths[0]
		} else {
			customBase = strings.TrimRight(strings.TrimSpace(values[0]), "/") + "/" + strings.TrimPrefix(p.Text(route, "path"), "/")
		}
		break
	}
	emails := map[string]any{}
	for _, u := range users {
		emails[p.Text(u, "id")] = u["email"]
	}
	for _, row := range rows {
		row["verificationRequired"] = p.Bool(policy, "enabled")
		row["verified"] = verified(row, policy)
		delete(row, "verificationVersion")
		row["email"] = emails[p.Text(row, "userId")]
		if p.Bool(policy, "enabled") && !p.Bool(row, "verified") {
			delete(row, "token")
			continue
		}
		base := "/api/v1/extensions/subscriptions/s/" + url.PathEscape(p.Text(row, "token"))
		if customBase != "" {
			base = customBase + "/" + url.PathEscape(p.Text(row, "token"))
		}
		row["clashUrl"] = base + "?format=clash"
		row["singboxUrl"] = base + "?format=sing-box"
		row["shareUrl"] = base
	}
	p.JSON(w, rows)
}

func (a *app) rotate(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.queryLinks([]p.Record{{"field": "id", "value": r.PathValue("id")}})
	if c.Err(w, e) {
		return
	}
	if len(rows) == 0 {
		p.Fail(w, 404, "订阅链接不存在")
		return
	}
	row := rows[0]
	if !c.Own(r, row) {
		p.Fail(w, 403, "无权修改此链接")
		return
	}
	row["token"] = p.ID() + p.ID()
	delete(row, "verificationVersion")
	delete(row, "boundUserAgent")
	delete(row, "boundClient")
	delete(row, "boundMajor")
	delete(row, "boundAt")
	if !c.Err(w, a.C.Put("links", row)) {
		p.JSON(w, true)
	}
}

func (a *app) deleteLink(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, err := a.queryLinks([]p.Record{{"field": "id", "value": r.PathValue("id")}})
	if c.Err(w, err) {
		return
	}
	if len(rows) == 0 {
		p.Fail(w, 404, "订阅链接不存在")
		return
	}
	if !c.Own(r, rows[0]) {
		p.Fail(w, 403, "无权删除此订阅")
		return
	}
	if !c.Err(w, a.C.Delete("links", p.Text(rows[0], "id"))) {
		p.JSON(w, true)
	}
}

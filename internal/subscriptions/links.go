package subscriptions

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/url"
)

func (a *app) queryLinks(filters []p.Record) ([]p.Record, error) {
	rows := []p.Record{}
	for {
		var page struct {
			Items []p.Record `json:"items"`
			Total int        `json:"total"`
		}
		if e := a.C.Request("POST", "/api/v1/runtime/query", p.Record{"collection": "links", "filters": filters, "offset": len(rows), "limit": 1000}, &page); e != nil {
			return nil, e
		}
		rows = append(rows, page.Items...)
		if len(rows) >= page.Total || len(page.Items) == 0 {
			return rows, nil
		}
	}
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
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients?userId="+url.QueryEscape(user), nil, &clients)) {
		return
	}
	if len(clients) == 0 {
		p.Fail(w, 400, "用户还没有可用访问账户")
		return
	}
	rows, e := a.queryLinks([]p.Record{{"field": "userId", "value": user}})
	if c.Err(w, e) {
		return
	}
	if len(rows) > 0 {
		p.Fail(w, 409, "此用户已经有订阅链接")
		return
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
	emails := map[string]any{}
	for _, u := range users {
		emails[p.Text(u, "id")] = u["email"]
	}
	for _, row := range rows {
		row["verificationRequired"] = p.Bool(policy, "enabled")
		row["verified"] = verified(row, policy)
		delete(row, "verificationVersion")
		row["email"] = emails[p.Text(row, "userId")]
		base := "/api/v1/extensions/subscriptions/s/" + url.PathEscape(p.Text(row, "token"))
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
	if !c.Err(w, a.C.Put("links", row)) {
		p.JSON(w, true)
	}
}

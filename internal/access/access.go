package access

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /accounts", a.accounts)
	a.Mux.HandleFunc("POST /accounts", a.save)
	a.Mux.HandleFunc("PATCH /accounts/{id}", a.save)
	a.Mux.HandleFunc("POST /accounts/{id}/reset", a.reset)
	a.Mux.HandleFunc("POST /accounts/{id}/suspend", a.suspend)
	a.Mux.HandleFunc("GET /internal/clients", a.clients)
	a.Mux.HandleFunc("GET /internal/accounts", a.allAccounts)
	a.Mux.HandleFunc("GET /internal/accounts/{user}", a.userAccount)
	a.Mux.HandleFunc("POST /internal/grant", a.grant)
	a.Mux.HandleFunc("POST /internal/revoke", a.revoke)
	a.View("accounts", a.view)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		rows, e := a.enrichedFor(subjectUser(r))
		if e != nil {
			return nil, e
		}
		used, quota, count := 0.0, 0.0, 0
		for _, row := range rows {
			if c.Own(r, row) {
				used += p.Number(row, "usedGiB")
				quota += p.Number(row, "quotaGiB")
				count++
			}
		}
		return []p.Metric{{Label: "访问账户", Value: count}, {Label: "本周期已用", Value: fmt.Sprintf("%.2f", used), Unit: "GB"}, {Label: "流量额度", Value: quota, Unit: "GB"}}, nil
	})
	a.Events(a.event)
	return a
}
func (a *app) accounts(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.enrichedFor(subjectUser(r))
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		if c.Own(r, row) {
			out = append(out, row)
		}
	}
	p.JSON(w, out)
}
func (a *app) save(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	var old p.Record
	if r.Method == "PATCH" {
		var e error
		old, e = a.Find("accounts", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		if b["userId"] != nil && b["userId"] != old["userId"] {
			p.Fail(w, 400, "访问账户所属用户不能修改")
			return
		}
		for k, v := range b {
			old[k] = v
		}
		b = old
	} else {
		delete(b, "id")
		if c.Err(w, c.Validate(b, "userId")) {
			return
		}
		users, e := a.C.Users()
		if c.Err(w, e) {
			return
		}
		exists := false
		for _, u := range users {
			if u["id"] == b["userId"] {
				exists = true
			}
		}
		if !exists {
			p.Fail(w, 400, "请选择有效用户")
			return
		}
		rows, e := a.C.List("accounts")
		if c.Err(w, e) {
			return
		}
		b["numericId"] = nextNumericID(rows)
		for _, row := range rows {
			if row["userId"] == b["userId"] {
				p.Fail(w, 409, "此用户已有访问账户")
				return
			}
		}
	}
	c.Defaults(b, old, p.Record{"status": "active", "quotaGiB": 100.0, "unlimited": false, "cycleDays": 30.0, "cycleStart": c.Now(), "credential": uuid(), "groups": "default"})
	if p.Number(b, "quotaGiB") < 0 || p.Number(b, "cycleDays") < 0 {
		p.Fail(w, 400, "流量与周期不能为负数")
		return
	}
	if !c.Allowed(p.Text(b, "status"), "active", "suspended") {
		p.Fail(w, 400, "状态无效")
		return
	}
	if !c.Err(w, a.C.Put("accounts", b)) {
		p.JSON(w, b)
	}
}
func uuid() string {
	s := p.ID()
	return s[:8] + "-" + s[8:12] + "-4" + s[13:16] + "-a" + s[17:20] + "-" + s[20:]
}
func (a *app) reset(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.Find("accounts", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	row["cycleStart"] = cycleNow()
	if !c.Err(w, a.C.Put("accounts", row)) {
		p.JSON(w, true)
	}
}
func (a *app) suspend(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.Find("accounts", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if p.Text(row, "status") == "active" {
		row["status"] = "suspended"
	} else {
		row["status"] = "active"
	}
	if !c.Err(w, a.C.Put("accounts", row)) {
		p.JSON(w, true)
	}
}
func (a *app) clients(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "nodes", "subscriptions") {
		return
	}
	rows, e := a.enrichedFor(r.URL.Query().Get("userId"))
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		if p.Bool(row, "effective") {
			out = append(out, clientAccount(row))
		}
	}
	p.JSON(w, out)
}
func (a *app) allAccounts(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "nodes") {
		return
	}
	rows, e := a.C.List("accounts")
	if !c.Err(w, e) {
		out := make([]p.Record, 0, len(rows))
		for _, row := range rows {
			out = append(out, p.Record{"id": row["id"], "userId": row["userId"], "numericId": row["numericId"]})
		}
		p.JSON(w, out)
	}
}
func (a *app) view(r *http.Request) (p.View, error) {
	admin := p.Admin(r)
	res := p.Resource{Key: "accounts", Title: "访问账户", Endpoint: "/accounts", Empty: "还没有访问账户", Columns: []p.Field{c.Field("email", "用户", "text", false), c.Field("status", "状态", "status", false), c.Field("usedGiB", "已用流量", "gib", false), c.Field("quotaGiB", "流量额度", "gib", false), c.Field("expiresAt", "到期时间", "datetime", false), c.GroupField("groups", "节点分组", false)}}
	if admin {
		res.CreateLabel = "添加访问账户"
		res.Editable = true
		res.Fields = []p.Field{c.UserField(a.C), c.Field("quotaGiB", "每周期流量（GB）", "number", true), c.Field("cycleDays", "重置周期（天，0 为不重置）", "number", true), c.Field("expiresAt", "到期时间（留空为长期）", "datetime", false), c.GroupField("groups", "可用节点分组", true), c.Select("status", "状态", "active|正常", "suspended|暂停"), c.Field("unlimited", "不限流量", "boolean", false)}
		res.Fields[1].Default = 100
		res.Fields[2].Default = 30
		res.Fields[4].Default = "default"
		res.Fields[5].Default = "active"
		res.Actions = []p.Action{{Key: "reset", Label: "重置流量", Confirm: "重置当前流量用量？"}, {Key: "suspend", Label: "切换状态", Confirm: "切换此账户的访问状态？"}}
	}
	return p.View{Title: "访问账户", Resources: []p.Resource{res}}, nil
}

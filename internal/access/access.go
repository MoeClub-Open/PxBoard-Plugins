package access

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"time"
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
	a.Mux.HandleFunc("POST /internal/grant", a.grant)
	a.Mux.HandleFunc("POST /internal/revoke", a.revoke)
	a.View("accounts", a.view)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		rows, e := a.enriched()
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
func (a *app) enriched() ([]p.Record, error) {
	rows, e := a.C.List("accounts")
	if e != nil {
		return nil, e
	}
	usage, e := a.C.List("usage")
	if e != nil {
		return nil, e
	}
	users, e := a.C.Users()
	if e != nil {
		return nil, e
	}
	maxID := 0.0
	for _, row := range rows {
		maxID = max(maxID, p.Number(row, "numericId"))
	}
	for _, row := range rows {
		if p.Number(row, "numericId") == 0 {
			maxID++
			row["numericId"] = maxID
			if e = a.C.Put("accounts", row); e != nil {
				return nil, e
			}
		}
	}
	totals := map[string]float64{}
	for _, u := range usage {
		totals[p.Text(u, "id")] = p.Number(u, "bytes")
	}
	for _, r := range rows {
		used := 0.0
		used = totals[p.Text(r, "userId")+"-"+p.Text(r, "cycleStart")] / 1073741824
		r["usedGiB"] = used
		userActive := false
		for _, u := range users {
			if u["id"] == r["userId"] {
				r["email"] = u["email"]
				userActive = p.Text(u, "status") == "active"
			}
		}
		unlimited := p.Bool(r, "unlimited")
		r["effective"] = userActive && p.Text(r, "status") == "active" && (unlimited || used < p.Number(r, "quotaGiB")) && (p.Text(r, "expiresAt") == "" || p.Text(r, "expiresAt") > c.Now())
	}
	return rows, nil
}
func (a *app) accounts(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.enriched()
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
		rows, _ := a.C.List("accounts")
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
	row["cycleStart"] = c.Now()
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
	if !c.Require(w, r, "nodes", "subscriptions", "sharing") {
		return
	}
	rows, e := a.enriched()
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		if p.Bool(row, "effective") {
			out = append(out, row)
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
		p.JSON(w, rows)
	}
}
func (a *app) event(e p.Event) error {
	switch e.Type {
	case "workspace.configured":
		if e.Source != "core" {
			return fmt.Errorf("工作空间事件来源无效")
		}
		mode, user := p.Text(e.Data, "mode"), p.Text(e.Data, "userId")
		if (mode != "personal" && mode != "sharing") || user == "" {
			return nil
		}
		rows, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		number := 0.0
		for _, row := range rows {
			if p.Text(row, "userId") == user {
				return nil
			}
			number = max(number, p.Number(row, "numericId"))
		}
		return a.C.Put("accounts", p.Record{"id": p.ID(), "userId": user, "numericId": number + 1, "credential": uuid(), "status": "active", "quotaGiB": 0, "unlimited": true, "cycleStart": c.Now(), "cycleDays": 0, "expiresAt": "", "groups": "default", "createdAt": c.Now()})
	case "traffic.reported":
		if e.Source != "nodes" {
			return fmt.Errorf("流量来源无效")
		}
		accounts, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		byUser := map[string]p.Record{}
		for _, row := range accounts {
			byUser[p.Text(row, "userId")] = row
		}
		usage, _ := e.Data["usage"].([]any)
		for _, raw := range usage {
			u, _ := raw.(map[string]any)
			user := p.Text(u, "userId")
			row := byUser[user]
			if row == nil || e.CreatedAt < p.Text(row, "cycleStart") {
				continue
			}
			operation := p.Text(e.Data, "nodeId") + "-" + p.Text(e.Data, "reportId") + "-" + user
			if err = a.C.Increment("usage", user+"-"+p.Text(row, "cycleStart"), "bytes", p.Number(u, "bytes")*p.Number(e.Data, "rate"), operation); err != nil {
				return err
			}
		}
		return nil
	case "access.suspend":
		if e.Source != "audit" {
			return fmt.Errorf("审计来源无效")
		}
		rows, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		for _, row := range rows {

			if row["userId"] == e.Data["userId"] {
				row["status"] = "suspended"
				row["suspendReason"] = e.Data["reason"]
				if err = a.C.Put("accounts", row); err != nil {
					return err
				}
			}
		}
	case "system.tick":
		rows, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if policy, ok := row["resetPolicy"].(map[string]any); ok {
				if p.Text(policy, "type") == "never" {
					continue
				}
				if p.Text(row, "expiresAt") != "" && p.Text(row, "expiresAt") <= c.Now() {
					continue
				}
				if p.Bool(policy, "onlyWhileLevelActive") && p.Text(row, "levelExpiresAt") <= c.Now() {
					continue
				}
				next, e := time.Parse(time.RFC3339, p.Text(policy, "nextAt"))
				if e != nil {
					start, e := time.Parse(time.RFC3339, p.Text(row, "cycleStart"))
					if e != nil {
						return e
					}
					next, e = c.NextReset(policy, start)
					if e != nil {
						return e
					}
				}
				if next.IsZero() {
					continue
				}
				for !next.After(time.Now()) {
					row["cycleStart"] = next.UTC().Format(time.RFC3339)
					if quota, exists := policy["quotaGiB"]; exists {
						row["quotaGiB"] = quota
					}
					next, e = c.NextReset(policy, next)
					if e != nil {
						return e
					}
				}
				policy["nextAt"] = next.UTC().Format(time.RFC3339)
				row["resetPolicy"] = policy
				if err = a.C.Put("accounts", row); err != nil {
					return err
				}
				continue
			}
			days := int(p.Number(row, "cycleDays"))
			if days == 0 {
				continue
			}
			start, err := time.Parse(time.RFC3339, p.Text(row, "cycleStart"))
			if err == nil && !start.AddDate(0, 0, days).After(time.Now()) {
				elapsed := int(time.Since(start).Hours()/24) / days
				row["cycleStart"] = start.AddDate(0, 0, elapsed*days).UTC().Format(time.RFC3339)
				if err = a.C.Put("accounts", row); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (a *app) view(r *http.Request) (p.View, error) {
	admin := p.Admin(r)
	res := p.Resource{Key: "accounts", Title: "访问账户", Endpoint: "/accounts", Empty: "还没有访问账户", Columns: []p.Field{c.Field("email", "用户", "text", false), c.Field("status", "状态", "status", false), c.Field("usedGiB", "已用流量", "gib", false), c.Field("quotaGiB", "流量额度", "gib", false), c.Field("expiresAt", "到期时间", "datetime", false), c.Field("groups", "节点分组", "text", false)}}
	if admin {
		res.CreateLabel = "添加访问账户"
		res.Editable = true
		res.Fields = []p.Field{c.UserField(a.C), c.Field("quotaGiB", "每周期流量（GB）", "number", true), c.Field("cycleDays", "重置周期（天，0 为不重置）", "number", true), c.Field("expiresAt", "到期时间（留空为长期）", "datetime", false), c.Field("groups", "可用节点分组", "text", true), c.Select("status", "状态", "active|正常", "suspended|暂停"), c.Field("unlimited", "不限流量", "boolean", false)}
		res.Fields[1].Default = 100
		res.Fields[2].Default = 30
		res.Fields[4].Default = "default"
		res.Fields[5].Default = "active"
		res.Actions = []p.Action{{Key: "reset", Label: "重置流量", Confirm: "重置当前流量用量？"}, {Key: "suspend", Label: "切换状态", Confirm: "切换此账户的访问状态？"}}
	}
	return p.View{Title: "访问账户", Resources: []p.Resource{res}}, nil
}

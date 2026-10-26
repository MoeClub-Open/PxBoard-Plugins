package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/url"
)

func (a *app) publicPlans(w http.ResponseWriter, r *http.Request) {
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if !businessEnabled(config) {
		p.JSON(w, []p.Record{})
		return
	}
	plans, e := a.C.List("plans")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, plan := range plans {
		if !p.Bool(plan, "enabled") || plan["visible"] == false {
			continue
		}
		item := p.Record{}
		for _, key := range []string{"id", "name", "description", "price", "currency", "durationUnit", "durationValue", "quotaGiB", "unlimited", "kind", "speedLimitMbps", "deviceLimit"} {
			if value, ok := plan[key]; ok {
				item[key] = value
			}
		}
		out = append(out, item)
	}
	p.JSON(w, out)
}

func planAvailable(plan, account p.Record) bool {
	if p.Bool(plan, "reviewRequired") {
		return false
	}
	if family := p.Text(plan, "planFamilyId"); family != "" && family == p.Text(account, "planFamilyId") {
		return plan["renewable"] != false
	}
	return p.Bool(plan, "enabled") && plan["visible"] != false
}

func (a *app) purchaseAccount(user string) (p.Record, error) {
	var account p.Record
	e := a.C.Call("access", "GET", "/internal/accounts/"+url.PathEscape(user), nil, &account)
	return account, e
}

func businessEnabled(config p.Record) bool {
	return p.Text(config, "workspaceMode") == "" || p.Text(config, "workspaceMode") == "business"
}

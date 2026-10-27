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
		periods := availablePeriods(plan, nil)
		if len(periods) == 0 {
			continue
		}
		item := p.Record{}
		for _, key := range []string{"id", "name", "description", "price", "currency", "durationUnit", "durationValue", "quotaGiB", "unlimited", "kind", "speedLimitMbps", "deviceLimit"} {
			if value, ok := plan[key]; ok {
				item[key] = value
			}
		}
		choices := []p.Record{}
		for _, period := range periods {
			choice := p.Record{"id": period["id"], "label": periodLabel(period)}
			effective := periodPlan(plan, period)
			for _, key := range []string{"price", "durationUnit", "durationValue", "kind"} {
				if value, ok := effective[key]; ok {
					choice[key] = value
				}
			}
			choices = append(choices, choice)
		}
		item["periods"] = choices
		out = append(out, item)
	}
	p.JSON(w, out)
}

func (a *app) purchaseAccount(user string) (p.Record, error) {
	var account p.Record
	e := a.C.Call("access", "GET", "/internal/accounts/"+url.PathEscape(user), nil, &account)
	return account, e
}

func businessEnabled(config p.Record) bool {
	return p.Text(config, "workspaceMode") == "" || p.Text(config, "workspaceMode") == "business"
}

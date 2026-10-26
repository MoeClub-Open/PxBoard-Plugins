package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
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
		if !p.Bool(plan, "enabled") {
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

func businessEnabled(config p.Record) bool {
	return p.Text(config, "workspaceMode") == "" || p.Text(config, "workspaceMode") == "business"
}

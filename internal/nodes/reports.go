package nodes

import p "github.com/MoeClub-Open/PxBoard/pkg/plugin"

func (a *app) hasReport(id string) (bool, error) {
	var page struct {
		Items []p.Record `json:"items"`
	}
	e := a.C.Request("POST", "/api/v1/runtime/query", p.Record{
		"collection": "reports", "filters": []p.Record{{"field": "id", "value": id}}, "limit": 1,
	}, &page)
	return len(page.Items) > 0, e
}

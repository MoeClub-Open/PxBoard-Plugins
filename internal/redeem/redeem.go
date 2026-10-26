package redeem

import (
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /codes", a.codes)
	a.Mux.HandleFunc("POST /codes", a.saveCode)
	a.Mux.HandleFunc("PATCH /codes/{id}", a.saveCode)
	a.Mux.HandleFunc("DELETE /codes/{id}", a.deleteCode)
	a.Mux.HandleFunc("GET /redeem", func(w http.ResponseWriter, r *http.Request) {
		if c.Member(w, r) {
			p.JSON(w, []p.Record{})
		}
	})
	a.Mux.HandleFunc("POST /redeem", a.redeem)
	a.Mux.HandleFunc("GET /claims", a.claims)
	a.View("codes", a.codesView)
	a.View("redeem", a.redeemView)
	a.View("claims", a.claimsView)
	a.Events(a.event)
	a.migrationAPI()
	return a
}

type queryResult struct {
	Items []p.Record `json:"items"`
	Total int        `json:"total"`
}

func (a *app) query(collection string, offset, limit int, filters ...p.Record) (queryResult, error) {
	var result queryResult
	err := a.C.Request("POST", "/api/v1/runtime/query", p.Record{"collection": collection, "filters": filters, "offset": offset, "limit": limit}, &result)
	return result, err
}

func equal(field string, value any) p.Record { return p.Record{"field": field, "value": value} }

func object(value any) p.Record {
	switch row := value.(type) {
	case p.Record:
		return row
	case map[string]any:
		return p.Record(row)
	}
	return nil
}

func (a *app) all(collection string, filters ...p.Record) ([]p.Record, error) {
	rows := []p.Record{}
	for offset := 0; ; offset += 500 {
		result, err := a.query(collection, offset, 500, filters...)
		if err != nil {
			return nil, err
		}
		rows = append(rows, result.Items...)
		if len(result.Items) == 0 || len(rows) >= result.Total {
			return rows, nil
		}
	}
}

func (a *app) find(collection, field, value string) (p.Record, error) {
	result, err := a.query(collection, 0, 1, equal(field, value))
	if err != nil || len(result.Items) == 0 {
		return nil, err
	}
	return result.Items[0], nil
}

func (a *app) walletAvailable() bool {
	var result struct {
		WalletCredits bool `json:"walletCredits"`
	}
	return a.C.Call("commerce", "GET", "/internal/capabilities", nil, &result) == nil && result.WalletCredits
}

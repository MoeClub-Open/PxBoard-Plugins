package affiliate

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /summary", a.summary)
	a.Mux.HandleFunc("GET /commissions", a.commissions)
	a.Mux.HandleFunc("GET /withdrawals", a.withdrawals)
	a.Mux.HandleFunc("POST /withdrawals", a.withdraw)
	a.Mux.HandleFunc("POST /withdrawals/{id}/{action}", a.withdrawAction)
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /profiles", a.profiles)
	a.Mux.HandleFunc("POST /profiles", a.saveProfile)
	a.Mux.HandleFunc("PATCH /profiles/{id}", a.saveProfile)
	a.View("referrals", a.referralView)
	a.View("settings", a.settingsView)
	a.Events(a.event)
	return a
}

func (a *app) query(collection string, filters ...p.Record) ([]p.Record, error) {
	return a.QueryAll(collection, filters...)
}

func equal(field string, value any) p.Record { return p.Record{"field": field, "value": value} }

func (a *app) record(collection, id string) (p.Record, error) {
	rows, e := a.query(collection, equal("id", id))
	if e != nil || len(rows) == 0 {
		return nil, e
	}
	return rows[0], nil
}

func (a *app) owned(collection string, r *http.Request) ([]p.Record, error) {
	if p.Admin(r) && r.URL.Query().Get("mine") != "1" {
		return a.query(collection)
	}
	return a.query(collection, equal("userId", p.Subject(r).ID))
}

func (a *app) event(e p.Event) error {
	switch e.Type {
	case "account.registered":
		if e.Source != "registration" && e.Source != "oauth" {
			return fmt.Errorf("注册事件来源无效")
		}
		return a.registerReferral(e.Data)
	case "order.fulfilled":
		if e.Source != "commerce" {
			return fmt.Errorf("订单事件来源无效")
		}
		return a.commission(e.Data)
	case "order.refunded":
		if e.Source != "commerce" {
			return fmt.Errorf("退款事件来源无效")
		}
		return a.reverse(e.Data)
	case "system.tick":
		if e.Source != "core" {
			return fmt.Errorf("后台任务来源无效")
		}
		return a.tick()
	}
	return nil
}

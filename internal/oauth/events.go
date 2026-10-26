package oauth

import (
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) event(event p.Event) error {
	if event.Type != "system.tick" || event.Source != "core" {
		return nil
	}
	if e := a.RetryRegistrations("bindings"); e != nil {
		return e
	}
	return a.RetryRegistrations("registrations")
}

func registrationRecord(row p.Record) p.Record {
	return p.Record{"userId": row["userId"], "referral": row["referral"], "registeredAt": row["registeredAt"]}
}

func (a *app) preserveRegistration(row p.Record) error {
	if p.Text(row, "registeredAt") == "" {
		return nil
	}
	rows, e := a.C.List("registrations")
	if e != nil {
		return e
	}
	for _, saved := range rows {
		if saved["userId"] == row["userId"] {
			return nil
		}
	}
	saved := registrationRecord(row)
	saved["id"] = row["userId"]
	saved["registrationPending"] = row["registrationPending"]
	saved["registrationEmittedAt"] = row["registrationEmittedAt"]
	return a.C.Put("registrations", saved)
}

func (a *app) registeredUsers(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "affiliate") {
		return
	}
	out := []p.Record{}
	seen := map[string]bool{}
	for _, collection := range []string{"registrations", "bindings"} {
		rows, e := a.C.List(collection)
		if c.Err(w, e) {
			return
		}
		for _, row := range rows {
			user := p.Text(row, "userId")
			if user != "" && p.Text(row, "registeredAt") != "" && !seen[user] {
				out = append(out, registrationRecord(row))
				seen[user] = true
			}
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, out)
}

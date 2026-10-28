package common

import (
	"fmt"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

// PublishRegistration keeps the durable source record pending until its event
// has entered the core queue. Consumers deduplicate by the stable userId.
func (a *App) PublishRegistration(collection string, row p.Record) error {
	if !p.Bool(row, "registrationPending") {
		return nil
	}
	user := p.Text(row, "userId")
	if user == "" {
		return fmt.Errorf("注册记录缺少用户标识")
	}
	data := p.Record{"userId": user, "registeredAt": row["registeredAt"]}
	if referral := p.Text(row, "referral"); referral != "" {
		data["referral"] = referral
	}
	row["registrationPending"], row["registrationEmittedAt"] = false, Now()
	return a.C.Commit("registration:"+user, []p.Write{{Collection: collection, Record: row}}, p.Emission{Type: "account.registered", Data: data})
}

func (a *App) RetryRegistrations(collection string) error {
	rows, e := a.QueryAll(collection, p.Record{"field": "registrationPending", "value": true})
	if e != nil {
		return e
	}
	for _, row := range rows {
		if e = a.PublishRegistration(collection, row); e != nil {
			return e
		}
	}
	return nil
}

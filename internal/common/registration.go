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
	if e := a.C.Emit("account.registered", data); e != nil {
		return e
	}
	row["registrationPending"], row["registrationEmittedAt"] = false, Now()
	return a.C.Put(collection, row)
}

func (a *App) RetryRegistrations(collection string) error {
	rows, e := a.C.List(collection)
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

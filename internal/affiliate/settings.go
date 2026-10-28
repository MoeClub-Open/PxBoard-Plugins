package affiliate

import (
	"math"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) config() (p.Record, error) {
	row, e := a.record("settings", "settings")
	if row == nil {
		row = p.Record{"id": "settings", "commissionPercent": 10.0, "commissionHoldDays": 7.0, "commissionFirstPurchaseOnly": true}
	}
	return row, e
}

func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.config()
	if c.Err(w, e) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, []p.Record{row})
		return
	}
	var values p.Record
	if c.Err(w, p.Decode(r, &values)) {
		return
	}
	for _, key := range []string{"commissionPercent", "commissionHoldDays", "commissionFirstPurchaseOnly"} {
		if value, ok := values[key]; ok {
			row[key] = value
		}
	}
	if p.Number(row, "commissionPercent") < 0 || p.Number(row, "commissionPercent") > 100 || p.Number(row, "commissionHoldDays") < 0 || math.Trunc(p.Number(row, "commissionHoldDays")) != p.Number(row, "commissionHoldDays") {
		p.Fail(w, 400, "返佣比例或冻结天数无效")
		return
	}
	if !c.Err(w, a.C.Put("settings", row)) {
		p.JSON(w, true)
	}
}

func (a *app) profile(user string) (p.Record, error) { return a.record("profiles", user) }

func (a *app) profiles(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.query("profiles")
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}

func (a *app) saveProfile(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var values p.Record
	if c.Err(w, p.Decode(r, &values)) {
		return
	}
	user := p.Text(values, "userId")
	if r.Method == "PATCH" {
		user = r.PathValue("id")
	}
	users, e := a.C.UsersByID(user)
	if c.Err(w, e) {
		return
	}
	found := false
	for _, row := range users {
		found = found || p.Text(row, "id") == user
	}
	if !found {
		p.Fail(w, 400, "用户不存在")
		return
	}
	row, e := a.profile(user)
	if c.Err(w, e) {
		return
	}
	if row == nil {
		row = p.Record{"id": user, "userId": user, "commissionMode": "system", "createdAt": c.Now()}
	}
	for _, key := range []string{"commissionPercent", "commissionMode"} {
		if value, ok := values[key]; ok {
			if key == "commissionPercent" && value == "" {
				value = nil
			}
			row[key] = value
		}
	}
	if !c.Allowed(p.Text(row, "commissionMode"), "system", "recurring", "firstPurchase") || p.Number(row, "commissionPercent") < 0 || p.Number(row, "commissionPercent") > 100 {
		p.Fail(w, 400, "返佣方式或比例无效")
		return
	}
	if !c.Err(w, a.C.Put("profiles", row)) {
		p.JSON(w, row)
	}
}

func terms(config, profile p.Record) (float64, bool) {
	percent, firstOnly := p.Number(config, "commissionPercent"), p.Bool(config, "commissionFirstPurchaseOnly")
	if profile["commissionPercent"] != nil {
		percent = p.Number(profile, "commissionPercent")
	}
	switch p.Text(profile, "commissionMode") {
	case "recurring":
		firstOnly = false
	case "firstPurchase":
		firstOnly = true
	}
	return percent, firstOnly
}

package affiliate

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) registerReferral(data p.Record) error {
	user, parent := p.Text(data, "userId"), strings.TrimSpace(p.Text(data, "referral"))
	if user == "" {
		return nil
	}
	processed, e := a.record("registrations", user)
	if e != nil || processed != nil {
		if e != nil {
			return e
		}
		return a.resumeOrders(user)
	}
	old, e := a.record("referrals", user)
	if e != nil {
		return e
	}
	users, e := a.C.Users()
	if e != nil {
		return e
	}
	foundUser, foundParent := false, false
	for _, row := range users {
		foundUser = foundUser || p.Text(row, "id") == user
		foundParent = foundParent || p.Text(row, "id") == parent
	}
	if !foundUser {
		return nil
	}
	created := p.Text(data, "registeredAt")
	if created == "" {
		created = c.Now()
	}
	if old == nil && foundParent && parent != user {
		if e = a.C.Put("referrals", p.Record{"id": user, "userId": user, "parentId": parent, "createdAt": created}); e != nil {
			return e
		}
	}
	if e = a.C.Put("registrations", p.Record{"id": user, "userId": user, "observedAt": c.Now()}); e != nil {
		return e
	}
	return a.resumeOrders(user)
}

func (a *app) resumeOrders(user string) error {
	orders, e := a.query("pendingOrders", equal("userId", user))
	if e != nil {
		return e
	}
	for _, order := range orders {
		if e = a.commission(p.Record{"orderId": order["id"]}); e != nil {
			return e
		}
		if e = a.C.Delete("pendingOrders", p.Text(order, "id")); e != nil {
			return e
		}
	}
	return nil
}

func (a *app) commission(data p.Record) error {
	id := p.Text(data, "orderId")
	if id == "" {
		return nil
	}
	seen, e := a.record("commissions", id)
	if e != nil || seen != nil {
		return e
	}
	state, e := a.record("orderStates", id)
	if e != nil || p.Bool(state, "refunded") {
		return e
	}
	var order p.Record
	if e = a.C.Call("commerce", "GET", "/internal/orders/"+url.PathEscape(id), nil, &order); e != nil {
		return e
	}
	if p.Text(order, "status") == "refunded" || p.Text(order, "status") == "refunding" {
		return a.reverse(data)
	}
	if p.Text(order, "status") != "fulfilled" || p.Text(order, "kind") == "recharge" || p.Number(order, "amount") <= 0 {
		return nil
	}
	referral, e := a.record("referrals", p.Text(order, "userId"))
	if e != nil {
		return e
	}
	if referral == nil {
		registration, e := a.record("registrations", p.Text(order, "userId"))
		if e != nil || registration != nil {
			return e
		}
		return a.C.Put("pendingOrders", p.Record{"id": id, "userId": order["userId"], "createdAt": c.Now()})
	}
	config, e := a.config()
	if e != nil {
		return e
	}
	referrer, e := a.profile(p.Text(referral, "parentId"))
	if e != nil {
		return e
	}
	percent, firstOnly := terms(config, referrer)
	if percent <= 0 {
		return nil
	}
	if firstOnly {
		first, e := a.firstPurchase(order)
		if e != nil || !first {
			return e
		}
	}
	amount := math.Floor(p.Number(order, "amount") * percent / 100)
	if amount == 0 {
		return nil
	}
	created := orderTime(order)
	days := int(p.Number(config, "commissionHoldDays"))
	status := "available"
	if days > 0 && created.AddDate(0, 0, days).After(time.Now()) {
		status = "pending"
	}
	return a.C.Put("commissions", p.Record{"id": id, "orderId": id, "userId": referral["parentId"], "buyerId": order["userId"], "amount": amount, "status": status, "availableAt": created.AddDate(0, 0, days).UTC().Format(time.RFC3339), "createdAt": created.UTC().Format(time.RFC3339)})
}

func orderTime(order p.Record) time.Time {
	for _, key := range []string{"paidAt", "fulfilledAt", "createdAt"} {
		if value, e := time.Parse(time.RFC3339, p.Text(order, key)); e == nil {
			return value
		}
	}
	return time.Now().UTC()
}

func (a *app) firstPurchase(order p.Record) (bool, error) {
	user := p.Text(order, "userId")
	profile, e := a.profile(user)
	if e != nil || p.Bool(profile, "hasPriorPurchase") {
		return false, e
	}
	var history p.Record
	if e = a.C.Call("commerce", "GET", "/internal/purchase-history/"+url.PathEscape(user), nil, &history); e != nil || p.Bool(history, "hasPriorPurchase") {
		return false, e
	}
	var orders []p.Record
	if e = a.C.Call("commerce", "GET", "/internal/orders?userId="+url.QueryEscape(user), nil, &orders); e != nil {
		return false, e
	}
	firstID, firstTime := p.Text(order, "id"), orderTime(order)
	for _, candidate := range orders {
		if p.Text(candidate, "kind") == "recharge" || c.Allowed(p.Text(candidate, "status"), "pending", "canceled") {
			continue
		}
		candidateID, candidateTime := p.Text(candidate, "id"), orderTime(candidate)
		if candidateTime.Before(firstTime) || candidateTime.Equal(firstTime) && candidateID < firstID {
			firstID, firstTime = candidateID, candidateTime
		}
	}
	return firstID == p.Text(order, "id"), nil
}

func (a *app) reverse(data p.Record) error {
	id := p.Text(data, "orderId")
	if id == "" {
		return nil
	}
	if e := a.C.Put("orderStates", p.Record{"id": id, "refunded": true, "updatedAt": c.Now()}); e != nil {
		return e
	}
	if e := a.C.Delete("pendingOrders", id); e != nil {
		return e
	}
	row, e := a.record("commissions", id)
	if e != nil || row == nil || p.Text(row, "status") == "reversed" {
		return e
	}
	row["status"], row["reversedAt"] = "reversed", c.Now()
	return a.C.Put("commissions", row)
}

func (a *app) tick() error {
	rows, e := a.query("commissions", equal("status", "pending"))
	if e != nil {
		return e
	}
	var failures []error
	for _, row := range rows {
		if available, e := time.Parse(time.RFC3339, p.Text(row, "availableAt")); e == nil && !available.After(time.Now()) {
			row["status"] = "available"
			if e = a.C.Put("commissions", row); e != nil {
				failures = append(failures, e)
			}
		}
	}
	withdrawals, e := a.query("withdrawals", equal("status", "approving"))
	if e != nil {
		return e
	}
	for _, row := range withdrawals {
		if e = a.creditWithdrawal(row); e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}

func (a *app) commissions(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.owned("commissions", r)
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}

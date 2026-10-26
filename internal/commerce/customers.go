package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"time"
)

func (a *app) customer(user string) (p.Record, error) {
	rows, e := a.C.List("customers")
	if e != nil {
		return nil, e
	}
	for _, row := range rows {
		if p.Text(row, "userId") == user {
			return row, nil
		}
	}
	return p.Record{}, nil
}

func (a *app) hadPurchase(user, currentOrder string) (bool, error) {
	customer, e := a.customer(user)
	if e != nil {
		return false, e
	}
	if p.Bool(customer, "hasPriorPurchase") {
		return true, nil
	}
	orders, e := a.C.List("orders")
	if e != nil {
		return false, e
	}
	for _, order := range orders {
		if p.Text(order, "userId") != user || p.Text(order, "id") == currentOrder || p.Text(order, "kind") == "recharge" {
			continue
		}
		if status := p.Text(order, "status"); status != "pending" && status != "canceled" {
			return true, nil
		}
	}
	return false, nil
}

func (a *app) commission(order p.Record) error {
	existing, e := a.C.List("commissions")
	if e != nil {
		return e
	}
	for _, row := range existing {
		if row["id"] == order["id"] {
			return nil
		}
	}
	config, e := a.config()
	if e != nil {
		return e
	}
	percent := p.Number(config, "commissionPercent")
	days := int(p.Number(config, "commissionHoldDays"))
	status := "available"
	if days > 0 {
		status = "pending"
	}
	referrals, e := a.C.List("referrals")
	if e != nil {
		return e
	}
	for _, ref := range referrals {
		if ref["userId"] != order["userId"] {
			continue
		}
		customer, e := a.customer(p.Text(ref, "parentId"))
		if e != nil {
			return e
		}
		if value := p.Number(customer, "commissionPercent"); value > 0 {
			percent = value
		}
		if percent <= 0 {
			return nil
		}
		firstOnly := p.Bool(config, "commissionFirstPurchaseOnly")
		switch p.Text(customer, "commissionMode") {
		case "recurring":
			firstOnly = false
		case "firstPurchase":
			firstOnly = true
		}
		if firstOnly {
			prior, e := a.hadPurchase(p.Text(order, "userId"), p.Text(order, "id"))
			if e != nil {
				return e
			}
			if prior {
				return nil
			}
		}
		return a.C.Put("commissions", p.Record{"id": order["id"], "orderId": order["id"], "userId": ref["parentId"], "buyerId": order["userId"], "amount": math.Floor(p.Number(order, "amount") * percent / 100), "status": status, "availableAt": time.Now().AddDate(0, 0, days).UTC().Format(time.RFC3339), "createdAt": c.Now()})
	}
	return nil
}

package commerce

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) quote(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	user := p.Subject(r).ID
	if p.Admin(r) && p.Text(b, "userId") != "" {
		user = p.Text(b, "userId")
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	row, err := a.quotePlan(user, b, config, time.Now())
	if !c.Err(w, err) {
		p.JSON(w, row)
	}
}

func (a *app) quotePlan(user string, b, config p.Record, now time.Time) (p.Record, error) {
	plan, period, err := a.selectPeriod(p.Text(b, "planId"), p.Text(b, "periodId"))
	if err != nil {
		return nil, err
	}
	account, err := a.purchaseAccount(user)
	if err != nil {
		return nil, err
	}
	if !periodAvailable(plan, period, account) {
		return nil, fmt.Errorf("此套餐暂不可购买或续费")
	}
	plan = periodPlan(plan, period)
	if account == nil && c.Allowed(p.Text(plan, "kind"), "traffic", "reset", "time") {
		return nil, fmt.Errorf("请先购买基础套餐，再使用补充套餐")
	}
	price := p.Number(plan, "price")
	row := p.Record{"planId": plan["id"], "periodId": period["id"], "periodLabel": periodLabel(period), "planName": p.Text(plan, "name") + " · " + periodLabel(period), "plan": plan, "kind": plan["kind"], "originalAmount": price, "amount": price, "discount": 0, "surplusAmount": 0, "surplusCredit": 0, "planFamilyId": plan["planFamilyId"]}
	if code := strings.TrimSpace(p.Text(b, "coupon")); code != "" {
		coupons, err := a.QueryAll("coupons", p.Record{"field": "code", "value": code})
		if err != nil {
			return nil, err
		}
		if len(coupons) == 0 {
			return nil, fmt.Errorf("优惠码无效或已到期")
		}
		coupon := coupons[0]
		orders, err := a.QueryAll("orders", p.Record{"field": "couponId", "value": coupon["id"]})
		if err != nil {
			return nil, err
		}
		if err = checkCoupon(coupon, plan, period, user, c.Now(), orders); err != nil {
			return nil, err
		}
		discount := p.Number(coupon, "discount")
		if p.Text(coupon, "type") == "percent" {
			discount = math.Round(price * discount / 100)
		}
		row["discount"], row["couponId"] = min(price, discount), coupon["id"]
	}
	customer, err := a.customer(user)
	if err != nil {
		return nil, err
	}
	if percent := p.Number(customer, "discountPercent"); percent > 0 {
		discount := math.Round(price * percent / 100)
		row["vipDiscount"] = discount
		row["discount"] = min(price, p.Number(row, "discount")+discount)
	}
	net := max(0, price-p.Number(row, "discount"))
	row["amount"], row["entitlementValue"] = net, net
	if p.Text(plan, "kind") == "subscription" {
		row["accountRevision"] = p.Text(account, "entitlementRevision")
		row["accountExists"] = account != nil
		family := p.Text(account, "planFamilyId")
		expiry, err := time.Parse(time.RFC3339, p.Text(account, "expiresAt"))
		active := account != nil && p.Text(account, "status") == "active" && (p.Text(account, "expiresAt") == "" || err == nil && expiry.After(now))
		changing := family != "" && active && (family != p.Text(plan, "planFamilyId") || (p.Text(plan, "durationUnit") == "permanent") != (p.Text(account, "expiresAt") == ""))
		row["replaceEntitlement"] = changing || family != p.Text(plan, "planFamilyId") || p.Text(plan, "durationUnit") == "permanent"
		if changing {
			if config["planChangeEnabled"] == false {
				return nil, fmt.Errorf("本站暂未开放更换套餐")
			}
			row["purchaseType"] = "change"
			if config["surplusEnabled"] != false {
				orders, err := a.QueryAll("orders", p.Record{"field": "userId", "value": user}, p.Record{"field": "status", "value": "fulfilled"})
				if err != nil {
					return nil, err
				}
				needsReceipts := false
				for _, order := range orders {
					if !p.Bool(order, "imported") && p.Text(order, "coverageStartAt") == "" {
						needsReceipts = true
					}
				}
				if needsReceipts {
					var receipts map[string]p.Record
					if err = a.C.Call("access", "GET", "/internal/billing/"+url.PathEscape(user), nil, &receipts); err != nil {
						return nil, err
					}
					normalizePaidTerms(orders, receipts)
				}
				value, ids := remainingValue(account, orders, now)
				row["surplusAmount"], row["surplusOrderIds"] = value, ids
				row["amount"], row["surplusCredit"] = max(0, net-value), max(0, value-net)
			}
		} else if active && family == p.Text(plan, "planFamilyId") {
			row["purchaseType"] = "renewal"
		} else {
			row["purchaseType"] = "new"
		}
	}
	return row, nil
}

func (a *app) availableOrderSlot(user string, config p.Record) error {
	orders, err := a.QueryAll("orders", p.Record{"field": "userId", "value": user}, p.Record{"field": "status", "op": "in", "value": []string{"pending", "paid", "refunding"}})
	if err != nil {
		return err
	}
	for _, row := range orders {
		if p.Bool(row, "imported") || p.Text(row, "kind") == "recharge" {
			continue
		}
		expired, err := a.expirePending(row, paymentTimeout(config), time.Now())
		if err != nil {
			return err
		}
		if !expired {
			return fmt.Errorf("请先完成或取消订单 %s，再创建新的套餐订单", p.Text(row, "number"))
		}
	}
	return nil
}

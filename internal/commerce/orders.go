package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
	"reflect"
	"strings"
	"time"
)

func (a *app) orders(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("orders", func(row p.Record) bool {
		return c.OwnList(r, row) && (r.URL.Query().Get("kind") == "" || p.Text(row, "kind") == r.URL.Query().Get("kind"))
	})
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) order(w http.ResponseWriter, r *http.Request) {
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
	requestID := p.Text(b, "requestId")
	intent := map[string]any{"planId": p.Text(b, "planId"), "periodId": p.Text(b, "periodId"), "kind": p.Text(b, "kind"), "amount": p.Number(b, "amount"), "coupon": strings.TrimSpace(p.Text(b, "coupon"))}
	if requestID != "" {
		orders, e := a.records("orders", p.Record{"field": "userId", "value": user}, p.Record{"field": "requestId", "value": requestID})
		if c.Err(w, e) {
			return
		}
		for _, order := range orders {
			if p.Text(order, "userId") == user && p.Text(order, "requestId") == requestID {
				previous, _ := order["request"].(map[string]any)
				if previous != nil && previous["periodId"] == nil {
					previous["periodId"] = ""
				}
				if !reflect.DeepEqual(previous, intent) {
					p.Fail(w, 409, "购买内容已变化，请重新确认后下单")
					return
				}
				p.JSON(w, order)
				return
			}
		}
	}
	id := p.ID()
	row := p.Record{"id": id, "number": "PX" + time.Now().Format("20060102150405") + id[:5], "userId": user, "status": "pending", "createdAt": c.Now(), "currency": "CNY", "amount": 0.0, "kind": "subscription"}
	if requestID != "" {
		row["requestId"] = requestID
		row["request"] = intent
	}
	if p.Text(b, "kind") == "recharge" {
		amount := p.Number(b, "amount")
		if amount < 100 || amount > 10000000 || math.Trunc(amount) != amount {
			p.Fail(w, 400, "充值金额不得低于最低限额，金额使用最小单位的整数")
			return
		}
		row["amount"] = amount
		row["kind"] = "recharge"
		row["planName"] = "余额充值"
	} else {
		plan, period, e := a.selectPeriod(p.Text(b, "planId"), p.Text(b, "periodId"))
		if c.Err(w, e) {
			return
		}
		account, e := a.purchaseAccount(user)
		if c.Err(w, e) {
			return
		}
		if !periodAvailable(plan, period, account) {
			p.Fail(w, 400, "此套餐暂不可购买或续费")
			return
		}
		plan = periodPlan(plan, period)
		if account == nil && c.Allowed(p.Text(plan, "kind"), "traffic", "reset", "time") {
			p.Fail(w, 400, "请先购买基础套餐，再使用补充套餐")
			return
		}
		row["planId"] = plan["id"]
		row["periodId"] = period["id"]
		row["periodLabel"] = periodLabel(period)
		row["planName"] = p.Text(plan, "name") + " · " + periodLabel(period)
		row["amount"] = plan["price"]
		row["plan"] = plan
		row["kind"] = plan["kind"]
		if code := strings.TrimSpace(p.Text(b, "coupon")); code != "" {
			coupons, e := a.C.List("coupons")
			if c.Err(w, e) {
				return
			}
			var coupon p.Record
			for _, v := range coupons {
				if v["code"] == code {
					coupon = v
				}
			}
			if coupon == nil || !p.Bool(coupon, "enabled") || p.Text(coupon, "expiresAt") != "" && p.Text(coupon, "expiresAt") < c.Now() {
				p.Fail(w, 400, "优惠码无效或已到期")
				return
			}
			orders, e := a.C.List("orders")
			if c.Err(w, e) {
				return
			}
			used := 0
			for _, o := range orders {
				if o["couponId"] == coupon["id"] && p.Text(o, "status") != "canceled" {
					used++
				}
			}
			if used >= int(p.Number(coupon, "limit")) {
				p.Fail(w, 400, "优惠码已用完")
				return
			}
			discount := p.Number(coupon, "discount")
			if p.Text(coupon, "type") == "percent" {
				discount = math.Round(p.Number(row, "amount") * discount / 100)
			}
			row["discount"] = discount
			row["amount"] = math.Max(0, p.Number(row, "amount")-discount)
			row["couponId"] = coupon["id"]
		}
		customer, e := a.customer(user)
		if c.Err(w, e) {
			return
		}
		if percent := p.Number(customer, "discountPercent"); percent > 0 {
			discount := math.Round(p.Number(plan, "price") * percent / 100)
			row["vipDiscount"] = discount
			row["discount"] = math.Min(p.Number(plan, "price"), p.Number(row, "discount")+discount)
			row["amount"] = math.Max(0, p.Number(plan, "price")-p.Number(row, "discount"))
		}
	}
	if !c.Err(w, a.C.Put("orders", row)) {
		p.JSON(w, row)
	}
}
func (a *app) orderAction(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.Find("orders", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if !c.Own(r, row) {
		p.Fail(w, 403, "无权操作此订单")
		return
	}
	action := r.PathValue("action")
	switch action {
	case "cancel":
		if p.Text(row, "status") != "pending" {
			p.Fail(w, 400, "只能取消待支付订单")
			return
		}
		debited, err := a.walletDebited(row)
		if c.Err(w, err) {
			return
		}
		if debited {
			p.Fail(w, 409, "余额已扣款，订单正在开通，不能取消")
			return
		}
		row["status"] = "canceled"
		e = a.C.Put("orders", row)
	case "pay":
		if p.Text(row, "status") == "fulfilled" {
			p.JSON(w, true)
			return
		}
		if p.Text(row, "status") != "pending" && p.Text(row, "status") != "paid" {
			p.Fail(w, 400, "订单状态不能支付")
			return
		}
		if p.Text(row, "kind") == "recharge" {
			p.Fail(w, 400, "余额充值需要使用支付渠道")
			return
		}
		if p.Text(row, "status") == "pending" {
			already, e2 := a.walletDebited(row)
			if c.Err(w, e2) {
				return
			}
			if !already {
				balance, e2 := a.balance(p.Text(row, "userId"))
				if c.Err(w, e2) {
					return
				}
				if balance < p.Number(row, "amount") {
					p.Fail(w, 400, "余额不足，请先充值或使用在线支付")
					return
				}
				e = a.C.Put("ledger", p.Record{"id": "order-" + p.Text(row, "id"), "userId": row["userId"], "amount": -p.Number(row, "amount"), "reason": "购买：" + p.Text(row, "planName"), "orderId": row["id"], "createdAt": c.Now()})
				if c.Err(w, e) {
					return
				}
			}
		}
		if p.Text(row, "status") == "pending" {
			row["method"] = "wallet"
		}
		e = a.paid(row)
	case "confirm":
		if !c.Require(w, r) {
			return
		}
		if p.Text(row, "status") != "pending" && p.Text(row, "status") != "paid" {
			p.Fail(w, 400, "只能确认待支付订单")
			return
		}
		if p.Text(row, "status") == "pending" {
			debited, err := a.walletDebited(row)
			if c.Err(w, err) {
				return
			}
			if debited {
				p.Fail(w, 409, "余额已扣款，订单正在开通，无需重复确认收款")
				return
			}
			row["method"] = "manual"
			row["confirmedBy"] = p.Subject(r).Email
		}
		e = a.paid(row)
	case "refund":
		if !c.Require(w, r) {
			return
		}
		if p.Text(row, "status") == "refunded" {
			p.JSON(w, row)
			return
		}
		if p.Text(row, "kind") == "recharge" {
			p.Fail(w, 400, "充值订单请通过原支付渠道退款，随后调整账本")
			return
		}
		if p.Text(row, "status") != "fulfilled" && p.Text(row, "status") != "refunding" {
			p.Fail(w, 400, "只能退回已完成订单")
			return
		}
		row["status"] = "refunding"
		if c.Err(w, a.C.Put("orders", row)) {
			return
		}
		e = a.refund(row)
	default:
		p.Fail(w, 404, "操作不存在")
		return
	}
	if !c.Err(w, e) {
		p.JSON(w, row)
	}
}

package commerce

import (
	"encoding/json"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
	"time"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.CRUD(planResource(), func(r *http.Request, b, old p.Record) error {
		c.Defaults(b, old, p.Record{"enabled": true, "visible": true, "renewable": true, "durationUnit": "month", "durationValue": 1.0, "timezone": "UTC", "quotaGiB": 100.0, "unlimited": false, "price": 1000.0, "groups": "default", "kind": "subscription", "currency": "CNY"})
		if p.Text(b, "planFamilyId") == "" {
			family := p.Text(old, "planFamilyId")
			if family == "" {
				family = p.Text(old, "id")
			}
			if family == "" {
				b["id"] = p.ID()
				family = p.Text(b, "id")
			}
			b["planFamilyId"] = family
		}
		if e := c.Validate(b, "name", "groups"); e != nil {
			return e
		}
		if p.Number(b, "price") < 0 || math.Trunc(p.Number(b, "price")) != p.Number(b, "price") || p.Number(b, "quotaGiB") < 0 {
			return fmt.Errorf("价格须为非负整数，流量不能为负数")
		}
		if !c.Allowed(p.Text(b, "kind"), "subscription", "time", "traffic", "reset") || p.Text(b, "currency") != "CNY" {
			return fmt.Errorf("请选择有效的套餐类型，金额币种须为人民币")
		}
		if _, e := time.LoadLocation(p.Text(b, "timezone")); e != nil {
			return fmt.Errorf("套餐时区无效")
		}
		if !c.Allowed(p.Text(b, "durationUnit"), "month", "day", "permanent") || p.Text(b, "durationUnit") != "permanent" && (p.Number(b, "durationValue") < 1 || math.Trunc(p.Number(b, "durationValue")) != p.Number(b, "durationValue")) {
			return fmt.Errorf("请输入有效的套餐期限")
		}
		if p.Bool(b, "enabled") {
			b["reviewRequired"] = false
		}
		return nil
	}, func(r *http.Request, row p.Record) bool {
		if p.Admin(r) {
			return true
		}
		account, e := a.purchaseAccount(p.Subject(r).ID)
		return e == nil && planAvailable(row, account)
	})
	a.CRUD(couponResource(), func(r *http.Request, b, old p.Record) error {
		c.Defaults(b, old, p.Record{"enabled": true, "limit": 100.0, "discount": 10.0, "type": "percent"})
		if e := c.Validate(b, "code"); e != nil {
			return e
		}
		if !c.Allowed(p.Text(b, "type"), "percent", "fixed") || p.Number(b, "discount") < 0 || p.Text(b, "type") == "percent" && p.Number(b, "discount") > 100 {
			return fmt.Errorf("优惠额度无效")
		}
		if p.Text(b, "type") == "fixed" && math.Trunc(p.Number(b, "discount")) != p.Number(b, "discount") || p.Number(b, "limit") < 0 || math.Trunc(p.Number(b, "limit")) != p.Number(b, "limit") {
			return fmt.Errorf("固定优惠金额须为整数分，可用次数须为非负整数")
		}
		return nil
	})
	a.CRUD(giftResource(), func(r *http.Request, b, old p.Record) error {
		c.Defaults(b, old, p.Record{"code": p.ID(), "amount": 1000.0, "status": "unused"})
		if p.Number(b, "amount") <= 0 || math.Trunc(p.Number(b, "amount")) != p.Number(b, "amount") {
			return fmt.Errorf("金额须为正整数")
		}
		if old != nil && p.Text(old, "status") != "unused" {
			return fmt.Errorf("已使用的充值卡不能修改")
		}
		return nil
	})
	a.Mux.HandleFunc("GET /orders", a.orders)
	a.Mux.HandleFunc("GET /public/plans", a.publicPlans)
	a.Mux.HandleFunc("POST /plans/{id}/details", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		plan, e := a.Find("plans", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		details, e := json.MarshalIndent(plan, "", "  ")
		if !c.Err(w, e) {
			p.JSON(w, p.Record{"title": "套餐信息", "details": string(details)})
		}
	})
	a.Mux.HandleFunc("POST /orders", a.order)
	a.Mux.HandleFunc("POST /orders/{id}/{action}", a.orderAction)
	a.Mux.HandleFunc("GET /ledger", a.ledger)
	a.Mux.HandleFunc("POST /ledger", a.adjust)
	a.Mux.HandleFunc("POST /gifts/redeem", a.redeem)
	a.Mux.HandleFunc("GET /referrals", a.referrals)
	a.Mux.HandleFunc("GET /affiliate/summary", a.affiliateSummary)
	a.Mux.HandleFunc("GET /withdrawals", a.withdrawals)
	a.Mux.HandleFunc("POST /withdrawals", a.withdraw)
	a.Mux.HandleFunc("POST /withdrawals/{id}/{action}", a.withdrawAction)
	a.Mux.HandleFunc("GET /internal/orders/{id}", a.internalOrder)
	a.Mux.HandleFunc("POST /register", a.register)
	a.Mux.HandleFunc("GET /_capabilities", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r, "core") {
			return
		}
		config, e := a.config()
		if !c.Err(w, e) {
			p.JSON(w, map[string]bool{"auth.registration": businessEnabled(config) && p.Bool(config, "registrationEnabled")})
		}
	})
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings/{id}", a.settings)
	a.View("settings", a.settingsView)
	a.View("plans", a.planView)
	a.View("orders", a.orderView)
	a.View("wallet", a.walletView)
	a.View("marketing", a.marketingView)
	a.View("referrals", a.referralView)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		rows, e := a.Filter("orders", func(row p.Record) bool { return c.Own(r, row) })
		if e != nil {
			return nil, e
		}
		total, pending := 0.0, 0
		for _, row := range rows {
			if p.Text(row, "status") == "fulfilled" && p.Text(row, "kind") != "recharge" {
				total += p.Number(row, "amount")
			}
			if p.Text(row, "status") == "pending" {
				pending++
			}
		}
		balance, e := a.balance(p.Subject(r).ID)
		return []p.Metric{{Label: "累计成交", Value: total / 100, Unit: "元"}, {Label: "待支付订单", Value: pending}, {Label: "我的余额", Value: balance / 100, Unit: "元"}}, e
	})
	a.Events(a.event)
	return a
}
func (a *app) event(e p.Event) error {
	switch e.Type {
	case "workspace.configured":
		if e.Source != "core" {
			return fmt.Errorf("工作空间事件来源无效")
		}
		config, err := a.config()
		if err != nil {
			return err
		}
		config["workspaceMode"] = p.Text(e.Data, "mode")
		return a.C.Put("settings", config)
	case "payment.succeeded":
		if e.Source != "payments" {
			return fmt.Errorf("支付事件来源无效")
		}
		return a.paymentSucceeded(e.Data)
	case "system.tick":
		return a.tick()
	}
	return nil
}
func (a *app) internalOrder(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "payments") {
		return
	}
	row, e := a.Find("orders", r.PathValue("id"))
	if !c.Err(w, e) {
		p.JSON(w, row)
	}
}

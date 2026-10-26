package commerce

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
	"net/url"
	"reflect"
)

func (a *app) affiliateSummary(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	user := p.Subject(r).ID
	available, e := a.available(user)
	if c.Err(w, e) {
		return
	}
	commissions, e := a.C.List("commissions")
	if c.Err(w, e) {
		return
	}
	total, pending := 0.0, 0.0
	for _, row := range commissions {
		if p.Text(row, "userId") != user {
			continue
		}
		status := p.Text(row, "status")
		if status == "reversed" || status == "historical" {
			continue
		}
		total += p.Number(row, "amount")
		if status == "pending" {
			pending += p.Number(row, "amount")
		}
	}
	referrals, e := a.C.List("referrals")
	if c.Err(w, e) {
		return
	}
	invited := 0
	for _, row := range referrals {
		if p.Text(row, "parentId") == user {
			invited++
		}
	}
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	customer, e := a.customer(user)
	if c.Err(w, e) {
		return
	}
	percent := p.Number(config, "commissionPercent")
	if value := p.Number(customer, "commissionPercent"); value > 0 {
		percent = value
	}
	firstOnly := p.Bool(config, "commissionFirstPurchaseOnly")
	switch p.Text(customer, "commissionMode") {
	case "recurring":
		firstOnly = false
	case "firstPurchase":
		firstOnly = true
	}
	mode := "recurring"
	if firstOnly {
		mode = "firstPurchase"
	}
	p.JSON(w, p.Record{"availableAmount": available, "pendingAmount": pending, "totalAmount": total, "invitedCount": invited, "commissionPercent": percent, "commissionHoldDays": config["commissionHoldDays"], "commissionMode": mode, "referralPath": "/register?ref=" + url.QueryEscape(user)})
}

func (a *app) referrals(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("commissions", func(row p.Record) bool { return c.Own(r, row) })
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) withdrawals(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("withdrawals", func(row p.Record) bool { return c.Own(r, row) })
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) available(user string) (float64, error) {
	rows, e := a.C.List("commissions")
	if e != nil {
		return 0, e
	}
	sum := 0.0
	for _, row := range rows {
		if row["userId"] == user && p.Text(row, "status") == "available" {
			sum += p.Number(row, "amount")
		}
	}
	withdrawals, e := a.C.List("withdrawals")
	if e != nil {
		return 0, e
	}
	for _, row := range withdrawals {
		if row["userId"] == user && p.Text(row, "status") != "rejected" && p.Text(row, "status") != "canceled" {
			sum -= p.Number(row, "amount")
		}
	}
	return sum, nil
}
func (a *app) withdraw(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	requestID := p.Text(b, "requestId")
	method := p.Text(b, "method")
	if method != "cash" {
		method = "wallet"
	}
	intent := map[string]any{"amount": p.Number(b, "amount"), "method": method, "payoutAccount": p.Text(b, "payoutAccount")}
	if requestID != "" {
		rows, e := a.C.List("withdrawals")
		if c.Err(w, e) {
			return
		}
		for _, row := range rows {
			if p.Text(row, "userId") == p.Subject(r).ID && p.Text(row, "requestId") == requestID {
				if !reflect.DeepEqual(row["request"], intent) {
					p.Fail(w, 409, "申请内容已变化，请重新确认后提交")
					return
				}
				p.JSON(w, row)
				return
			}
		}
	}
	amount := p.Number(b, "amount")
	sum, e := a.available(p.Subject(r).ID)
	if c.Err(w, e) {
		return
	}
	if amount < 100 || math.Trunc(amount) != amount || amount > sum {
		p.Fail(w, 400, "可申请金额不足，最低 1 元")
		return
	}
	if method == "cash" && p.Text(b, "payoutAccount") == "" {
		p.Fail(w, 400, "请填写收款账户")
		return
	}
	b = p.Record{"id": p.ID(), "userId": p.Subject(r).ID, "amount": amount, "method": method, "payoutAccount": b["payoutAccount"], "status": "pending", "createdAt": c.Now()}
	if requestID != "" {
		b["requestId"] = requestID
		b["request"] = intent
	}
	if !c.Err(w, a.C.Put("withdrawals", b)) {
		p.JSON(w, b)
	}
}
func (a *app) withdrawAction(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.Find("withdrawals", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if r.PathValue("action") == "cancel" {
		if p.Text(row, "userId") != p.Subject(r).ID || p.Text(row, "status") != "pending" {
			p.Fail(w, 400, "只能撤回本人待审批的申请")
			return
		}
		row["status"] = "canceled"
		if !c.Err(w, a.C.Put("withdrawals", row)) {
			p.JSON(w, true)
		}
		return
	}
	if !c.Require(w, r) {
		return
	}
	if r.PathValue("action") == "paid" {
		if p.Text(row, "method") != "cash" || p.Text(row, "status") != "approved" {
			p.Fail(w, 400, "只能确认已审批的现金提现")
			return
		}
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if c.Err(w, c.Validate(b, "reference")) {
			return
		}
		row["status"] = "paid"
		row["reference"] = b["reference"]
		row["actor"] = p.Subject(r).Email
		row["paidAt"] = c.Now()
		if !c.Err(w, a.C.Put("withdrawals", row)) {
			p.JSON(w, true)
		}
		return
	}
	if p.Text(row, "status") != "pending" && p.Text(row, "status") != "approving" {
		p.Fail(w, 400, "申请已处理")
		return
	}
	switch r.PathValue("action") {
	case "approve":
		available, err := a.available(p.Text(row, "userId"))
		if c.Err(w, err) {
			return
		}
		if p.Text(row, "status") == "pending" && available < 0 {
			p.Fail(w, 409, "可结算佣金因退款等变动已不足，请先核对或拒绝申请")
			return
		}
		if p.Text(row, "method") == "cash" {
			row["status"] = "approved"
			row["actor"] = p.Subject(r).Email
			if !c.Err(w, a.C.Put("withdrawals", row)) {
				p.JSON(w, true)
			}
			return
		}
		row["status"] = "approving"
		if c.Err(w, a.C.Put("withdrawals", row)) {
			return
		}
		e = a.C.Put("ledger", p.Record{"id": "commission-" + p.Text(row, "id"), "userId": row["userId"], "amount": row["amount"], "reason": "佣金转入余额", "createdAt": c.Now()})
		if e == nil {
			row["status"] = "approved"
			row["actor"] = p.Subject(r).Email
			e = a.C.Put("withdrawals", row)
		}
	case "reject":
		if p.Text(row, "status") != "pending" {
			p.Fail(w, 400, "转账处理中不能拒绝")
			return
		}
		row["status"] = "rejected"
		e = a.C.Put("withdrawals", row)
	default:
		e = fmt.Errorf("操作不存在")
	}
	if !c.Err(w, e) {
		p.JSON(w, true)
	}
}

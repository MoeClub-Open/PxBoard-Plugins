package affiliate

import (
	"fmt"
	"log"
	"maps"
	"math"
	"net/http"
	"reflect"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) available(user string) (float64, error) {
	commissions, e := a.query("commissions", equal("userId", user), equal("status", "available"))
	if e != nil {
		return 0, e
	}
	total := 0.0
	for _, row := range commissions {
		total += p.Number(row, "amount")
	}
	withdrawals, e := a.query("withdrawals", equal("userId", user))
	for _, row := range withdrawals {
		if !c.Allowed(p.Text(row, "status"), "rejected", "canceled") {
			total -= p.Number(row, "amount")
		}
	}
	return total, e
}

func (a *app) withdrawals(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.owned("withdrawals", r)
	if !c.Err(w, e) {
		for _, row := range rows {
			row["statusLabel"] = withdrawalStatus(row)
		}
		p.JSON(w, rows)
	}
}

func (a *app) withdraw(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var values p.Record
	if c.Err(w, p.Decode(r, &values)) {
		return
	}
	user, requestID, method := p.Subject(r).ID, p.Text(values, "requestId"), p.Text(values, "method")
	if !c.Allowed(method, "cash", "wallet") {
		p.Fail(w, 400, "请选择转入余额或现金提现")
		return
	}
	intent := map[string]any{"amount": p.Number(values, "amount"), "method": method, "payoutAccount": p.Text(values, "payoutAccount")}
	if requestID != "" {
		rows, e := a.query("withdrawals", equal("userId", user), equal("requestId", requestID))
		if c.Err(w, e) {
			return
		}
		if len(rows) > 0 {
			if !reflect.DeepEqual(rows[0]["request"], intent) {
				p.Fail(w, 409, "结算内容已变化，请重新确认后提交")
				return
			}
			if method == "wallet" && c.Allowed(p.Text(rows[0], "status"), "pending", "approving") && c.Err(w, a.startWalletTransfer(rows[0])) {
				return
			}
			p.JSON(w, rows[0])
			return
		}
	}
	amount := p.Number(values, "amount")
	available, e := a.available(user)
	if c.Err(w, e) {
		return
	}
	minimum := 1.0
	if method == "cash" {
		minimum = 100
	}
	if amount < minimum || math.Trunc(amount) != amount || amount > available {
		message := "可转入金额不足"
		if method == "cash" {
			message = "可提现金额不足"
		}
		p.Fail(w, 400, message)
		return
	}
	if method == "cash" && p.Text(values, "payoutAccount") == "" {
		p.Fail(w, 400, "请填写收款账户")
		return
	}
	row := p.Record{"id": p.ID(), "userId": user, "amount": amount, "method": method, "payoutAccount": p.Text(values, "payoutAccount"), "status": "pending", "createdAt": c.Now()}
	if method == "wallet" {
		row["status"] = "approving"
	}
	if requestID != "" {
		row["requestId"], row["request"] = requestID, intent
	}
	if !c.Err(w, a.C.Put("withdrawals", row)) {
		if method == "wallet" {
			_ = a.startWalletTransfer(row)
		}
		p.JSON(w, row)
	}
}

func (a *app) withdrawAction(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.record("withdrawals", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if row == nil {
		p.Fail(w, 404, "结算记录不存在")
		return
	}
	action := r.PathValue("action")
	if action == "transfer" {
		if p.Text(row, "method") != "wallet" || p.Text(row, "userId") != p.Subject(r).ID {
			p.Fail(w, 403, "只能转入本人的佣金")
			return
		}
		if p.Text(row, "status") != "approved" && !c.Allowed(p.Text(row, "status"), "pending", "approving") {
			p.Fail(w, 400, "此记录不能继续转入")
			return
		}
		if p.Text(row, "status") != "approved" && c.Err(w, a.startWalletTransfer(row)) {
			return
		}
		p.JSON(w, row)
		return
	}
	if action == "cancel" {
		if p.Text(row, "userId") != p.Subject(r).ID || p.Text(row, "status") != "pending" {
			p.Fail(w, 400, "只能取消本人尚未执行的结算")
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
	switch action {
	case "paid":
		if p.Text(row, "method") != "cash" || p.Text(row, "status") != "approved" {
			p.Fail(w, 400, "只能确认已审批的现金提现")
			return
		}
		var body p.Record
		if c.Err(w, p.Decode(r, &body)) || c.Err(w, c.Validate(body, "reference")) {
			return
		}
		row["status"], row["reference"], row["paidAt"], row["actor"] = "paid", body["reference"], c.Now(), p.Subject(r).Email
		e = a.C.Put("withdrawals", row)
	case "approve":
		if p.Text(row, "method") != "cash" {
			p.Fail(w, 400, "转入余额由用户自助完成，无需审批")
			return
		}
		if p.Text(row, "status") != "pending" {
			p.Fail(w, 400, "申请已处理")
			return
		}
		available, err := a.available(p.Text(row, "userId"))
		if c.Err(w, err) {
			return
		}
		if available < 0 {
			p.Fail(w, 409, "可结算佣金因退款等变动已不足，请先核对或拒绝申请")
			return
		}
		row["actor"] = p.Subject(r).Email
		row["status"] = "approved"
		e = a.C.Put("withdrawals", row)
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

func (a *app) startWalletTransfer(row p.Record) error {
	if p.Text(row, "status") == "pending" {
		available, e := a.available(p.Text(row, "userId"))
		if e != nil {
			return e
		}
		if available < 0 {
			return fmt.Errorf("可用佣金因退款等变动已不足，请取消原记录后重新转入")
		}
		row["status"] = "approving"
		if e = a.C.Put("withdrawals", row); e != nil {
			return e
		}
	}
	if e := a.creditWithdrawal(row); e != nil {
		log.Printf("commission wallet transfer %s pending: %v", p.Text(row, "id"), e)
	}
	return nil
}

func (a *app) creditWithdrawal(row p.Record) error {
	if e := a.C.Call("commerce", "POST", "/internal/wallet/credits", p.Record{"userId": row["userId"], "amount": row["amount"], "operationId": "commission-" + p.Text(row, "id"), "reason": "佣金转入余额"}, nil); e != nil {
		return e
	}
	completed := maps.Clone(row)
	completed["status"], completed["approvedAt"] = "approved", c.Now()
	if e := a.C.Put("withdrawals", completed); e != nil {
		return e
	}
	row["status"], row["approvedAt"] = completed["status"], completed["approvedAt"]
	return nil
}

func withdrawalStatus(row p.Record) string {
	switch p.Text(row, "status") {
	case "pending":
		if p.Text(row, "method") == "wallet" {
			return "待转入"
		}
		return "待审核"
	case "approving":
		return "转入中"
	case "approved":
		if p.Text(row, "method") == "wallet" {
			return "已入余额"
		}
		return "待打款"
	case "paid":
		return "已打款"
	case "rejected":
		return "已拒绝"
	case "canceled":
		return "已取消"
	}
	return p.Text(row, "status")
}

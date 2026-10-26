package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
	"strings"
)

func (a *app) balance(user string) (float64, error) {
	rows, e := a.C.List("ledger")
	sum := 0.0
	for _, row := range rows {
		if row["userId"] == user {
			sum += p.Number(row, "amount")
		}
	}
	return sum, e
}
func (a *app) ledger(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("ledger", func(row p.Record) bool { return c.Own(r, row) })
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) adjust(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if c.Err(w, c.Validate(b, "userId", "reason")) {
		return
	}
	amount := p.Number(b, "amount")
	if amount == 0 || math.Trunc(amount) != amount {
		p.Fail(w, 400, "金额须为非零整数分")
		return
	}
	balance, e := a.balance(p.Text(b, "userId"))
	if c.Err(w, e) {
		return
	}
	if balance+amount < 0 {
		p.Fail(w, 400, "余额不能为负数")
		return
	}
	row := p.Record{"id": p.ID(), "userId": b["userId"], "amount": amount, "reason": b["reason"], "actor": p.Subject(r).Email, "createdAt": c.Now()}
	if !c.Err(w, a.C.Put("ledger", row)) {
		p.JSON(w, row)
	}
}
func (a *app) redeem(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	rows, e := a.C.List("gifts")
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		if row["code"] != b["code"] {
			continue
		}
		if p.Text(row, "status") != "unused" && row["userId"] != p.Subject(r).ID || p.Text(row, "expiresAt") != "" && p.Text(row, "expiresAt") < c.Now() {
			p.Fail(w, 400, "充值卡已使用或已到期")
			return
		}
		row["status"] = "used"
		row["userId"] = p.Subject(r).ID
		if c.Err(w, a.C.Put("gifts", row)) {
			return
		}
		if !c.Err(w, a.C.Put("ledger", p.Record{"id": "gift-" + p.Text(row, "id"), "userId": p.Subject(r).ID, "amount": row["amount"], "reason": "充值卡兑换", "createdAt": c.Now()})) {
			p.JSON(w, true)
		}
		return
	}
	p.Fail(w, 400, "充值卡不存在")
}
func (a *app) register(w http.ResponseWriter, r *http.Request) {
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if !businessEnabled(config) || !p.Bool(config, "registrationEnabled") {
		p.Fail(w, 403, "本站暂未开放注册")
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	var user p.Record
	if c.Err(w, a.C.Request("POST", "/api/v1/runtime/users", b, &user)) {
		return
	}
	if ref := strings.TrimSpace(p.Text(b, "referral")); ref != "" {
		users, e := a.C.Users()
		if c.Err(w, e) {
			return
		}
		for _, u := range users {
			if u["id"] == ref && u["id"] != user["id"] {
				if c.Err(w, a.C.Put("referrals", p.Record{"id": user["id"], "userId": user["id"], "parentId": ref, "createdAt": c.Now()})) {
					return
				}
				break
			}
		}
	}
	p.JSON(w, p.Record{"message": "注册成功，可以登录", "id": user["id"]})
}

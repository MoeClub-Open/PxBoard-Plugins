package commerce

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"math"
	"net/http"
)

func (a *app) balance(user string) (float64, error) {
	rows, e := a.records("ledger", p.Record{"field": "userId", "value": user})
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

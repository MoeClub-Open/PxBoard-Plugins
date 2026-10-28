package commerce

import (
	"math"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) config() (p.Record, error) {
	rows, e := a.QueryAll("settings", p.Record{"field": "id", "value": "settings"})
	if e != nil || len(rows) == 0 {
		return p.Record{"id": "settings"}, e
	}
	return rows[0], nil
}

func (a *app) customer(user string) (p.Record, error) {
	rows, e := a.QueryAll("customers", p.Record{"field": "userId", "value": user})
	if e != nil || len(rows) == 0 {
		return p.Record{}, e
	}
	return rows[0], nil
}

func (a *app) internalOrders(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "affiliate") {
		return
	}
	if r.URL.Query().Get("userId") == "" {
		p.Fail(w, 400, "缺少用户标识")
		return
	}
	rows, e := a.QueryAll("orders", p.Record{"field": "userId", "value": r.URL.Query().Get("userId")})
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		item := p.Record{}
		for _, key := range []string{"id", "userId", "status", "kind", "createdAt", "paidAt", "fulfilledAt", "amount", "planName"} {
			if value, ok := row[key]; ok {
				item[key] = value
			}
		}
		out = append(out, item)
	}
	p.JSON(w, out)
}

func (a *app) internalCapabilities(w http.ResponseWriter, r *http.Request) {
	if c.Require(w, r, "affiliate", "redeem") {
		p.JSON(w, p.Record{"walletCredits": true})
	}
}

func (a *app) internalPurchaseHistory(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "affiliate") {
		return
	}
	row, e := a.customer(r.PathValue("userId"))
	if !c.Err(w, e) {
		p.JSON(w, p.Record{"hasPriorPurchase": p.Bool(row, "hasPriorPurchase")})
	}
}

func (a *app) internalCredit(w http.ResponseWriter, r *http.Request) {
	if !p.Service(r, "affiliate") && !p.Service(r, "redeem") {
		p.Fail(w, 403, "无权写入钱包")
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) || c.Err(w, c.Validate(b, "userId", "operationId", "reason")) {
		return
	}
	amount := p.Number(b, "amount")
	if amount <= 0 || math.Trunc(amount) != amount {
		p.Fail(w, 400, "入账金额须为正整数分")
		return
	}
	id := "external:" + p.Subject(r).Plugin + ":" + p.Text(b, "operationId")
	existing, e := a.QueryAll("ledger", p.Record{"field": "id", "value": id})
	if c.Err(w, e) {
		return
	}
	if len(existing) > 0 {
		row := existing[0]
		if p.Text(row, "userId") != p.Text(b, "userId") || p.Number(row, "amount") != amount {
			p.Fail(w, 409, "相同入账标识的用户或金额不一致")
			return
		}
		p.JSON(w, p.Record{"credited": true, "duplicate": true})
		return
	}
	users, e := a.C.UsersByID(p.Text(b, "userId"))
	if c.Err(w, e) {
		return
	}
	found := false
	for _, user := range users {
		if p.Text(user, "id") == p.Text(b, "userId") {
			found = true
			break
		}
	}
	if !found {
		p.Fail(w, 404, "入账用户不存在")
		return
	}
	row := p.Record{"id": id, "userId": b["userId"], "amount": amount, "reason": b["reason"], "sourcePlugin": p.Subject(r).Plugin, "operationId": b["operationId"], "createdAt": c.Now()}
	if !c.Err(w, a.C.Put("ledger", row)) {
		p.JSON(w, p.Record{"credited": true, "duplicate": false})
	}
}

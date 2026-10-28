package commerce

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) paid(row p.Record) error {
	row["status"] = "paid"
	if row["paidAt"] == nil {
		row["paidAt"] = c.Now()
	}
	if e := a.C.Put("orders", row); e != nil {
		return e
	}
	return a.fulfill(row)
}
func (a *app) fulfill(row p.Record) error {
	if p.Text(row, "kind") == "recharge" {
		if e := a.C.Put("ledger", p.Record{"id": "recharge-" + p.Text(row, "id"), "userId": row["userId"], "amount": row["amount"], "reason": "余额充值", "orderId": row["id"], "createdAt": c.Now()}); e != nil {
			return e
		}
	} else {
		plan, _ := row["plan"].(map[string]any)
		if plan == nil {
			return fmt.Errorf("套餐快照不存在")
		}
		grant := p.Record{"grantId": row["id"], "userId": row["userId"]}
		for _, key := range []string{"quotaGiB", "unlimited", "durationUnit", "durationValue", "timezone", "groups", "kind", "accessLevel", "speedLimitMbps", "deviceLimit", "resetPolicy", "planFamilyId"} {
			if plan[key] != nil {
				grant[key] = plan[key]
			}
		}
		if e := a.C.Call("access", "POST", "/internal/grant", grant, nil); e != nil {
			return e
		}
	}
	row["status"] = "fulfilled"
	row["fulfilledAt"] = c.Now()
	return a.C.Commit("order-fulfilled:"+p.Text(row, "id"), []p.Write{{Collection: "orders", Record: row}}, p.Emission{Type: "order.fulfilled", Data: p.Record{"orderId": row["id"], "userId": row["userId"], "amount": row["amount"], "planName": row["planName"], "kind": row["kind"], "paidAt": row["paidAt"], "fulfilledAt": row["fulfilledAt"]}})
}
func (a *app) refund(row p.Record) error {
	if p.Text(row, "kind") == "recharge" {
		row["status"] = "fulfilled"
		return a.C.Put("orders", row)
	}
	if e := a.C.Call("access", "POST", "/internal/revoke", p.Record{"grantId": row["id"], "userId": row["userId"]}, nil); e != nil {
		return e
	}
	if e := a.C.Put("ledger", p.Record{"id": "refund-" + p.Text(row, "id"), "userId": row["userId"], "amount": row["amount"], "reason": "订单退回余额", "orderId": row["id"], "createdAt": c.Now()}); e != nil {
		return e
	}
	row["status"] = "refunded"
	row["refundedAt"] = c.Now()
	return a.C.Commit("order-refunded:"+p.Text(row, "id"), []p.Write{{Collection: "orders", Record: row}}, p.Emission{Type: "order.refunded", Data: p.Record{"orderId": row["id"], "userId": row["userId"], "amount": row["amount"], "refundedAt": row["refundedAt"]}})
}

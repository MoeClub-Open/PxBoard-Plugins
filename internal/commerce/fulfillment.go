package commerce

import (
	"errors"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"time"
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
		if revision, ok := row["accountRevision"]; ok {
			grant["expectedRevision"], grant["expectedAccountExists"] = revision, row["accountExists"]
			grant["replaceEntitlement"] = row["replaceEntitlement"]
		}
		var result p.Record
		if e := a.C.Call("access", "POST", "/internal/grant", grant, &result); e != nil {
			var failure *p.APIError
			if errors.As(e, &failure) && failure.Status == 409 {
				return a.returnStalePayment(row)
			}
			return e
		}
		grants, _ := result["grants"].(map[string]any)
		receipt, _ := grants[p.Text(row, "id")].(map[string]any)
		if receipt != nil {
			start, _ := time.Parse(time.RFC3339, p.Text(receipt, "createdAt"))
			if !p.Bool(row, "replaceEntitlement") {
				if old, err := time.Parse(time.RFC3339, p.Text(receipt, "expiresAt")); err == nil && old.After(start) {
					start = old
				}
			}
			row["coverageStartAt"], row["coverageEndAt"] = start.UTC().Format(time.RFC3339), receipt["grantedExpiresAt"]
		}
	}
	row["status"] = "fulfilled"
	row["fulfilledAt"] = c.Now()
	writes, err := a.creditOrderWrites(row, false)
	if err != nil {
		return err
	}
	if p.Number(row, "surplusCredit") > 0 {
		writes = append(writes, p.Write{Collection: "ledger", Record: p.Record{"id": "surplus-" + p.Text(row, "id"), "userId": row["userId"], "amount": row["surplusCredit"], "reason": "更换套餐折抵余款", "orderId": row["id"], "createdAt": c.Now()}})
	}
	writes = append(writes, p.Write{Collection: "orders", Record: row})
	return a.C.Commit("order-fulfilled:"+p.Text(row, "id"), writes, p.Emission{Type: "order.fulfilled", Data: p.Record{"orderId": row["id"], "orderNumber": row["number"], "userId": row["userId"], "amount": row["amount"], "planName": row["planName"], "kind": row["kind"], "paidAt": row["paidAt"], "fulfilledAt": row["fulfilledAt"]}})
}
func (a *app) refund(row p.Record) error {
	if p.Text(row, "kind") == "recharge" {
		row["status"] = "fulfilled"
		return a.C.Put("orders", row)
	}
	net := p.Number(row, "amount") - p.Number(row, "surplusCredit")
	balance, err := a.balance(p.Text(row, "userId"))
	if err != nil {
		return err
	}
	if balance+net < 0 {
		return fmt.Errorf("折抵余款已使用，余额不足以撤销该次换购")
	}
	if e := a.C.Call("access", "POST", "/internal/revoke", p.Record{"grantId": row["id"], "userId": row["userId"]}, nil); e != nil {
		return e
	}

	row["status"] = "refunded"
	row["refundedAt"] = c.Now()
	writes, err := a.creditOrderWrites(row, true)
	if err != nil {
		return err
	}
	writes = append(writes, p.Write{Collection: "orders", Record: row}, p.Write{Collection: "ledger", Record: p.Record{"id": "refund-" + p.Text(row, "id"), "userId": row["userId"], "amount": net, "reason": "订单退款并撤销折抵余款", "orderId": row["id"], "createdAt": c.Now()}})
	return a.C.Commit("order-refunded:"+p.Text(row, "id"), writes, p.Emission{Type: "order.refunded", Data: p.Record{"orderId": row["id"], "userId": row["userId"], "amount": row["amount"], "refundedAt": row["refundedAt"]}})
}

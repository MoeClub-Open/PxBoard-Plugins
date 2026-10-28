package commerce

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) creditOrderWrites(row p.Record, restore bool) ([]p.Write, error) {
	writes := []p.Write{}
	for _, id := range surplusIDs(row) {
		old, err := a.Find("orders", id)
		if err != nil {
			return nil, err
		}
		if restore {
			if p.Text(old, "creditedBy") != p.Text(row, "id") {
				return nil, fmt.Errorf("折抵订单的结算记录不一致")
			}
			old["status"] = "fulfilled"
			delete(old, "creditedBy")
			delete(old, "creditedAt")
		} else {
			if p.Text(old, "status") != "fulfilled" && p.Text(old, "creditedBy") != p.Text(row, "id") {
				return nil, fmt.Errorf("该套餐权益已在其他订单中结算")
			}
			old["status"], old["creditedBy"], old["creditedAt"] = "credited", row["id"], c.Now()
		}
		writes = append(writes, p.Write{Collection: "orders", Record: old})
	}
	return writes, nil
}

func (a *app) validOrderAccount(row p.Record) (bool, error) {
	if _, ok := row["accountRevision"]; !ok {
		return true, nil
	}
	account, err := a.purchaseAccount(p.Text(row, "userId"))
	return (account != nil) == p.Bool(row, "accountExists") && p.Text(account, "entitlementRevision") == p.Text(row, "accountRevision"), err
}

// Payment may arrive after a redemption or an administrator changed access.
// Keep those new entitlements and return the received money to the wallet.
func (a *app) returnStalePayment(row p.Record) error {
	row["status"], row["cancelReason"], row["refundedAt"] = "refunded", "entitlements_changed", c.Now()
	return a.C.Commit("stale-payment:"+p.Text(row, "id"), []p.Write{
		{Collection: "orders", Record: row},
		{Collection: "ledger", Record: p.Record{"id": "stale-payment-" + p.Text(row, "id"), "userId": row["userId"], "amount": row["amount"], "orderId": row["id"], "reason": "账户权益已变化，付款退回余额", "createdAt": c.Now()}},
	})
}

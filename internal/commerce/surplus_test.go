package commerce

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"testing"
	"time"
)

func TestRemainingPaidTermsExcludeGiftsAndSettledOrders(t *testing.T) {
	now := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	account := p.Record{"planFamilyId": "a", "expiresAt": "2027-10-01T00:00:00Z"} // gifted extension
	orders := []p.Record{
		{"id": "paid", "status": "fulfilled", "kind": "subscription", "planFamilyId": "a", "entitlementValue": 3000, "coverageStartAt": "2026-10-01T00:00:00Z", "coverageEndAt": "2026-10-31T00:00:00Z"},
		{"id": "settled", "status": "credited", "kind": "subscription", "planFamilyId": "a", "entitlementValue": 99999, "coverageStartAt": "2026-10-01T00:00:00Z", "coverageEndAt": "2027-10-31T00:00:00Z"},
		{"id": "other", "status": "fulfilled", "kind": "subscription", "planFamilyId": "b", "entitlementValue": 99999, "coverageStartAt": "2026-10-01T00:00:00Z", "coverageEndAt": "2027-10-31T00:00:00Z"},
	}
	value, ids := remainingValue(account, orders, now)
	if value != 1500 || len(ids) != 1 || ids[0] != "paid" {
		t.Fatal(value, ids)
	}
	account["expiresAt"] = "2026-10-20T00:00:00Z"
	value, _ = remainingValue(account, orders, now)
	if value != 400 {
		t.Fatal("shortened entitlement overvalued", value)
	}
}

func TestImportedAndPermanentSurplus(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	account := p.Record{"planSourceId": "8", "expiresAt": "2026-11-01T00:00:00Z", "quotaGiB": 100, "usedGiB": 25}
	order := p.Record{"id": "legacy", "status": "fulfilled", "kind": "subscription", "imported": true, "periodId": "monthly", "amount": 2000, "balanceAmount": 500, "createdAt": "2026-10-01T00:00:00Z", "sourceContent": map[string]any{"plan_id": float64(8), "surplus_amount": float64(1000), "surplus_credit": float64(500)}}
	value, ids := remainingValue(account, []p.Record{order}, now)
	if value != 1500 || len(ids) != 1 {
		t.Fatal("imported payment/balance/credit lost", value, ids)
	}
	order["sourceContent"] = map[string]any{"plan_id": "8", "surplus_amount": "1000", "surplus_credit": "500"}
	value, _ = remainingValue(account, []p.Record{order}, now)
	if value != 1500 {
		t.Fatal("SQL-exported numeric strings lost", value)
	}
	account["expiresAt"] = ""
	order["periodId"] = "onetime"
	value, _ = remainingValue(account, []p.Record{order}, now)
	if value != 2250 {
		t.Fatal("permanent traffic surplus", value)
	}
	account["usedGiB"] = 100
	value, _ = remainingValue(account, []p.Record{order}, now)
	if value != 0 {
		t.Fatal("exhausted traffic still credited", value)
	}
}

func TestEarlierNativeOrdersUseGrantReceipts(t *testing.T) {
	order := p.Record{"id": "before", "kind": "subscription", "status": "fulfilled", "amount": 3100, "plan": map[string]any{"kind": "subscription", "planFamilyId": "a", "durationUnit": "month", "durationValue": float64(1)}}
	orders := []p.Record{order}
	normalizePaidTerms(orders, map[string]p.Record{"before": {"createdAt": "2026-10-01T00:00:00Z"}})
	value, _ := remainingValue(p.Record{"planFamilyId": "a", "expiresAt": "2026-11-01T00:00:00Z"}, orders, time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC))
	if value != 1500 {
		t.Fatal("earlier native paid term omitted", value)
	}
}

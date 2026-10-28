package commerce

import (
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestCouponRestrictionsIncludeMigratedUsage(t *testing.T) {
	coupon := p.Record{"id": "code", "enabled": true, "limit": 2, "perUserLimit": 1, "planFamilyIds": []any{"family"}, "periodIds": []any{"period"}}
	plan, period := p.Record{"planFamilyId": "family"}, p.Record{"id": "period"}
	orders := []p.Record{{"couponId": "code", "userId": "first", "status": "fulfilled", "imported": true}}
	if err := checkCoupon(coupon, plan, period, "second", "2026-10-28T00:00:00Z", orders); err != nil {
		t.Fatal("remaining use was lost", err)
	}
	if checkCoupon(coupon, plan, period, "first", "2026-10-28T00:00:00Z", orders) == nil {
		t.Fatal("migrated use did not count towards personal limit")
	}
	if checkCoupon(coupon, p.Record{"planFamilyId": "other"}, period, "second", "2026-10-28T00:00:00Z", orders) == nil {
		t.Fatal("plan restriction lost")
	}
	orders = append(orders, p.Record{"couponId": "code", "userId": "second", "status": "pending"})
	if checkCoupon(coupon, plan, period, "third", "2026-10-28T00:00:00Z", orders) == nil {
		t.Fatal("reserved use did not count towards total limit")
	}
}

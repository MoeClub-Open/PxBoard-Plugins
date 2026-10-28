package migration

import (
	"sort"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (m *mapper) v2Coupons() {
	used := map[string]int64{}
	for _, order := range m.S.Rows("v2_order") {
		if integer(order, "status") != 0 && integer(order, "status") != 2 && m.Users[text(order, "user_id")] {
			used[text(order, "coupon_id")]++
		}
	}
	table, rows := m.table("v2_coupon")
	for _, row := range rows {
		kind := map[int64]string{1: "fixed", 2: "percent"}[integer(row, "type")]
		if kind == "" {
			m.fail("优惠码记录 %s 的优惠方式无法识别", text(row, "id"))
			continue
		}
		families, periodIDs := []string{}, []string{}
		for _, id := range groupIDs(row["limit_plan_ids"]) {
			families = append(families, m.id("plan-family", id))
		}
		periods := groupIDs(row["limit_period"])
		for _, period := range periods {
			if canonical, ok := legacyPeriods[period]; ok {
				period = canonical
			}
			for _, plan := range m.S.Rows("v2_plan") {
				periodIDs = append(periodIDs, m.id("plan", text(plan, "id")+":"+period))
			}
		}
		sort.Strings(periodIDs)
		limit := int64(0)
		enabled := truth(row, "show")
		if row["limit_use"] != nil {
			limit = integer(row, "limit_use") + used[text(row, "id")]
			if integer(row, "limit_use") <= 0 {
				enabled = false
			}
		}
		coupon := p.Record{"code": text(row, "code"), "name": text(row, "name"), "type": kind, "discount": m.money(row, "value", false), "limit": limit, "perUserLimit": integer(row, "limit_use_with_user"), "enabled": enabled, "startsAt": m.timeValue(row, "started_at"), "expiresAt": m.timeValue(row, "ended_at"), "planFamilyIds": families, "periodIds": periodIDs, "createdAt": m.created(row, "created_at"), "source": m.ref(row, table), "sourceContent": row}
		if row["limit_use_with_user"] != nil && integer(row, "limit_use_with_user") == 0 {
			coupon["enabled"] = false
		}
		m.doc("commerce", "coupons", m.id("coupon", text(row, "id")), coupon)
	}
}

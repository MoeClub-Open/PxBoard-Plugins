package migration

import p "github.com/MoeClub-Open/PxBoard/pkg/plugin"

func (m *mapper) v2Affiliate(rows []Row, table string, priorBuyers map[string]bool) {
	omitted := 0
	for _, row := range rows {
		amount := m.money(row, "commission_balance", false)
		if !m.Options.IncludeAffiliate {
			if amount != 0 {
				omitted++
			}
			continue
		}
		id := m.id("user", text(row, "id"))
		if parent := m.userID(row, "invite_user_id"); parent != "" {
			m.doc("affiliate", "referrals", id, p.Record{"userId": id, "parentId": parent, "createdAt": m.created(row, "created_at")})
		}
		if amount != 0 {
			m.doc("affiliate", "commissions", m.id("opening-commission", text(row, "id")), p.Record{"userId": id, "amount": amount, "status": "available", "createdAt": m.S.ExportedAt, "source": m.ref(row, table)})
		}
		modeValue := text(row, "commission_type")
		if modeValue == "" {
			modeValue = "0"
		}
		mode := map[string]string{"0": "system", "1": "recurring", "2": "firstPurchase"}[modeValue]
		if mode == "" {
			m.fail("用户 %s 的返佣方式无法识别", text(row, "email"))
		}
		commission := number(row, "commission_rate")
		if commission < 0 || commission > 100 {
			m.fail("用户 %s 的返佣比例无效", text(row, "email"))
		}
		profile := p.Record{"userId": id, "commissionMode": mode, "hasPriorPurchase": priorBuyers[text(row, "id")], "source": m.ref(row, table)}
		if commission > 0 {
			profile["commissionPercent"] = commission
		}
		m.doc("affiliate", "profiles", id, profile)
	}
	if omitted > 0 {
		m.fail("源站有 %d 个用户持有非零可用佣金；请安装并启用推广插件，然后新建任务勾选“同时导入推广关系与佣金”，不能仅归档这笔财务权益", omitted)
	}
}

func (m *mapper) ssAffiliate(rows []Row) {
	if !m.Options.IncludeAffiliate {
		return
	}
	priorBuyers := map[string]bool{}
	for _, order := range m.S.Rows("order") {
		if status := text(order, "status"); status == "activated" || status == "expired" {
			priorBuyers[text(order, "user_id")] = true
		}
	}
	for _, bought := range m.S.Rows("bought") {
		priorBuyers[text(Row{"user": first(bought, "user_id", "userid")}, "user")] = true
	}
	for _, row := range rows {
		id := m.id("user", text(row, "id"))
		if parent := m.userID(row, "ref_by"); parent != "" {
			m.doc("affiliate", "referrals", id, p.Record{"userId": id, "parentId": parent, "createdAt": m.created(row, "reg_date")})
		}
		m.doc("affiliate", "profiles", id, p.Record{"userId": id, "commissionMode": "system", "hasPriorPurchase": priorBuyers[text(row, "id")]})
	}
	m.warning("SSPanel 已结算的历史返利包含在用户原余额中，仅归档返利日志，不重复创建佣金或余额。")
}

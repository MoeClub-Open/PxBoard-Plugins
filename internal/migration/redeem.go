package migration

import (
	"fmt"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (m *mapper) v2Redeem() {
	templates, plans := map[string]Row{}, map[string]Row{}
	_, rows := m.table("v2_gift_card_template")
	for _, row := range rows {
		templates[text(row, "id")] = row
	}
	for _, row := range m.S.Rows("v2_plan") {
		plans[text(row, "id")] = row
	}
	usageTable, usages := m.table("v2_gift_card_usage")
	byCode := map[string][]Row{}
	for _, row := range usages {
		byCode[text(row, "code_id")] = append(byCode[text(row, "code_id")], row)
	}
	table, rows := m.table("v2_gift_card_code")
	for _, row := range rows {
		template := templates[text(row, "template_id")]
		rewards, limits, conditions := object(template, "rewards"), object(template, "limits"), object(template, "conditions")
		code := p.Record{"code": text(row, "code"), "name": text(template, "name"), "kind": "balance", "amount": 0, "limit": integer(row, "max_usage"), "perUserLimit": 0, "historicalUses": 0, "enabled": truth(template, "status") && integer(row, "status") <= 1, "expiresAt": m.timeValue(row, "expires_at"), "createdAt": m.created(row, "created_at"), "redemptionGroup": m.id("gift-template", text(row, "template_id")), "groupPerUserLimit": integer(limits, "max_use_per_user"), "cooldownHours": number(limits, "cooldown_hours"), "source": m.ref(row, table), "sourceContent": row, "sourceTemplate": template}
		if code["name"] == "" {
			code["name"] = "兑换码"
		}
		if integer(row, "max_usage") == 1 {
			code["perUserLimit"] = 1
		}
		issues := []string{}
		if template == nil || integer(row, "max_usage") < 1 {
			issues = append(issues, "缺少模板或有效使用次数")
		}
		if integer(template, "type") == 3 || nonempty(rewards["random_rewards"]) || len(object(template, "special_config")) > 0 {
			issues = append(issues, "随机或时段奖励需要核对")
		}
		for key, value := range rewards {
			switch key {
			case "balance", "plan_id", "plan_validity_days", "expire_days", "transfer_enable", "reset_package", "random_rewards":
			default:
				if nonempty(value) {
					issues = append(issues, "未识别的奖励："+key)
				}
			}
		}
		if number(rewards, "balance") > 0 {
			code["amount"] = m.money(rewards, "balance", false)
		}
		if planID := text(rewards, "plan_id"); planID != "" && planID != "0" {
			plan := plans[planID]
			if plan == nil {
				issues = append(issues, "奖励套餐不存在")
			} else {
				code["kind"], code["durationUnit"], code["durationValue"] = "access", "unchanged", 0
				if days := integer(rewards, "plan_validity_days"); days > 0 {
					code["durationUnit"], code["durationValue"] = "day", days
				}
				code["quotaGiB"], code["groups"] = number(plan, "transfer_enable"), m.group(text(plan, "group_id"))
				code["speedLimitMbps"], code["deviceLimit"] = integer(plan, "speed_limit"), integer(plan, "device_limit")
				code["planFamilyId"], code["preserveUsage"] = m.id("plan-family", planID), true
				code["timezone"], code["resetPolicy"] = m.S.Timezone, m.resetPolicy(plan, Row{})
			}
		} else {
			kinds := 0
			if number(rewards, "transfer_enable") > 0 {
				code["kind"], code["quotaGiB"] = "traffic", number(rewards, "transfer_enable")/1073741824
				kinds++
			}
			if integer(rewards, "expire_days") > 0 {
				code["kind"], code["durationUnit"], code["durationValue"] = "time", "day", integer(rewards, "expire_days")
				kinds++
			}
			if truth(rewards, "reset_package") {
				code["kind"] = "reset"
				kinds++
			}
			if kinds > 1 {
				issues = append(issues, "多种访问奖励需要核对")
			}
			code["requiresPlan"] = kinds > 0
		}
		code["requiresInactiveAccount"] = integer(template, "type") == 2
		if allowed, exists := conditions["allowed_plans"]; exists {
			families := []string{}
			for _, id := range groupIDs(allowed) {
				families = append(families, m.id("plan-family", id))
			}
			code["restrictPlanFamilies"], code["allowedPlanFamilies"] = true, families
		}
		for key, value := range conditions {
			if key != "allowed_plans" && key != "disallowed_plans" && nonempty(value) {
				issues = append(issues, "未识别的领取条件："+key)
			}
		}
		for key, value := range limits {
			if key != "max_use_per_user" && key != "cooldown_hours" && nonempty(value) {
				issues = append(issues, "未识别的次数限制："+key)
			}
		}
		if len(issues) > 0 {
			code["enabled"], code["reviewRequired"], code["reviewReason"] = false, true, strings.Join(issues, "；")
			m.warning("兑换码记录 %s 保留全部配置并停用：%s", text(row, "id"), code["reviewReason"])
		}
		converted := int64(0)
		for _, usage := range byCode[text(row, "id")] {
			user := m.userID(usage, "user_id")
			if user == "" {
				continue
			}
			converted++
			m.doc("redeem", "claims", m.id("gift-usage", text(usage, "id")), p.Record{"codeId": m.id("gift-code", text(row, "id")), "userId": user, "name": code["name"], "kind": code["kind"], "summary": giftSummary(object(usage, "rewards_given"), plans), "status": "redeemed", "redemptionGroup": code["redemptionGroup"], "createdAt": m.created(usage, "created_at"), "redeemedAt": m.created(usage, "created_at"), "source": m.ref(usage, usageTable), "sourceContent": usage})
		}
		if converted > integer(row, "usage_count") {
			m.fail("兑换码记录 %s 的使用次数少于实际记录数", text(row, "id"))
		}
		code["historicalUses"] = max(int64(0), integer(row, "usage_count")-converted)
		m.doc("redeem", "codes", m.id("gift-code", text(row, "id")), code)
	}
}

func nonempty(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return fmt.Sprint(value) != "" && fmt.Sprint(value) != "0" && fmt.Sprint(value) != "false"
	}
}

func giftSummary(rewards Row, plans map[string]Row) string {
	items := []string{}
	if amount := number(rewards, "balance"); amount > 0 {
		items = append(items, fmt.Sprintf("余额 %.2f", amount/100))
	}
	if id := text(rewards, "plan_id"); id != "" {
		name := text(plans[id], "name")
		if name == "" {
			name = "套餐 " + id
		}
		if days := integer(rewards, "plan_validity_days"); days > 0 {
			name += fmt.Sprintf(" · %d天", days)
		}
		items = append(items, name)
	}
	if number(rewards, "transfer_enable") > 0 {
		items = append(items, fmt.Sprintf("追加 %g GB", number(rewards, "transfer_enable")/1073741824))
	}
	if integer(rewards, "expire_days") > 0 {
		items = append(items, fmt.Sprintf("延长 %d天", integer(rewards, "expire_days")))
	}
	if truth(rewards, "reset_package") {
		items = append(items, "重置流量")
	}
	return strings.Join(items, " + ")
}

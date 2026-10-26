package migration

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strings"
)

var periods = map[string]struct {
	months int
	label  string
	kind   string
}{"monthly": {1, "月付", "subscription"}, "quarterly": {3, "季付", "subscription"}, "half_yearly": {6, "半年付", "subscription"}, "yearly": {12, "年付", "subscription"}, "two_yearly": {24, "两年付", "subscription"}, "three_yearly": {36, "三年付", "subscription"}, "onetime": {0, "一次性", "subscription"}, "reset_traffic": {0, "重置流量", "reset"}}
var legacyPeriods = map[string]string{"month_price": "monthly", "quarter_price": "quarterly", "half_year_price": "half_yearly", "year_price": "yearly", "two_year_price": "two_yearly", "three_year_price": "three_yearly", "onetime_price": "onetime", "reset_price": "reset_traffic"}

func (m *mapper) v2Plans() {
	table, rows := m.table("v2_plan")
	for _, row := range rows {
		prices := object(row, "prices")
		converted := 0
		yuan := len(prices) > 0
		if !yuan {
			for old, new := range legacyPeriods {
				if value, ok := row[old]; ok && value != nil {
					prices[new] = value
				}
			}
		}
		for period, value := range prices {
			if value == nil {
				continue
			}
			definition, ok := periods[period]
			if !ok {
				m.fail("套餐 %s 含未知计费周期 %s", text(row, "name"), period)
				continue
			}
			group := m.group(text(row, "group_id"))
			id := m.id("plan", text(row, "id")+":"+period)
			enabled := truth(row, "show")
			if _, exists := row["sell"]; exists {
				enabled = truth(row, "sell")
			}
			plan := p.Record{"name": text(row, "name") + " · " + definition.label, "price": m.money(Row{"price": value}, "price", yuan), "durationUnit": "month", "durationValue": definition.months, "quotaGiB": number(row, "transfer_enable"), "groups": group, "kind": definition.kind, "currency": "CNY", "enabled": enabled, "renew": row["renew"], "capacityLimit": integer(row, "capacity_limit"), "speedLimitMbps": integer(row, "speed_limit"), "deviceLimit": integer(row, "device_limit"), "source": m.ref(row, table), "sourcePeriod": period, "createdAt": m.created(row, "created_at")}
			plan["unlimited"] = false
			plan["visible"] = row["show"] == nil || truth(row, "show")
			plan["renewable"] = row["renew"] == nil || truth(row, "renew")
			plan["sort"] = integer(row, "sort")
			plan["timezone"] = m.S.Timezone
			plan["planFamilyId"] = m.id("plan-family", text(row, "id"))
			plan["sourceContent"] = row
			if period == "onetime" {
				plan["durationUnit"] = "permanent"
				plan["durationValue"] = 0
				plan["resetPolicy"] = p.Record{"type": "never"}
			} else if period == "reset_traffic" {
				plan["resetPolicy"] = p.Record{"type": "never"}
			} else {
				plan["resetPolicy"] = m.resetPolicy(row, Row{})
			}
			if integer(row, "capacity_limit") > 0 {
				plan["enabled"] = false
				plan["reviewRequired"] = true
				m.warning("套餐 %s 的限售规则已保留，核对后再开售", p.Text(plan, "name"))
			}
			m.doc("commerce", "plans", id, plan)
			converted++
		}
		if converted == 0 {
			m.doc("commerce", "plans", m.id("plan", text(row, "id")+":unpriced"), p.Record{"name": row["name"], "enabled": false, "reviewRequired": true, "sourceContent": row, "source": m.ref(row, table), "quotaGiB": number(row, "transfer_enable"), "groups": m.group(text(row, "group_id"))})
			m.warning("套餐 %s 没有定价，已保留为停售套餐", text(row, "name"))
		}
	}
}
func (m *mapper) ssPlans() {
	table, rows := m.table("product")
	if rows == nil {
		table, rows = m.table("shop", "new_shop")
	}
	for _, row := range rows {
		content := object(row, "content")
		if len(content) == 0 {
			content = object(row, "content_extra")
		}
		kind := "subscription"
		if text(row, "type") == "bandwidth" {
			kind = "traffic"
		}
		if text(row, "type") == "time" {
			kind = "time"
		}
		days := integer(content, "time")
		if days == 0 {
			days = integer(content, "class_expire")
		}
		if days == 0 {
			days = integer(content, "days")
		}
		quota := number(content, "bandwidth")
		if quota == 0 {
			quota = number(content, "transfer_enable")
		}
		if quota == 0 {
			quota = number(content, "traffic")
		}
		if len(content) == 0 {
			m.warning("SSPanel 商品 %s 没有可识别的内容配置，保留原商品并停售", text(row, "id"))
		}
		if len(object(row, "limit")) > 0 {
			m.warning("商品 %s 的购买限制已保留为规则，开售前请核对", text(row, "name"))
		}
		name := text(row, "name")
		if name == "" {
			name = text(row, "shop_name")
		}
		plan := p.Record{"name": name, "price": m.money(row, "price", true), "durationUnit": "day", "durationValue": days, "quotaGiB": quota, "groups": m.group(text(content, "node_group")), "accessLevel": integer(content, "class"), "kind": kind, "currency": "CNY", "enabled": truth(row, "status"), "capacityLimit": integer(row, "stock"), "purchaseRules": object(row, "limit"), "sourceType": text(row, "type"), "source": m.ref(row, table), "sourceContent": content, "createdAt": m.created(row, "create_time")}
		plan["sourceRecord"] = row
		plan["planFamilyId"] = m.id("plan-family", text(row, "id"))
		plan["unlimited"] = false
		plan["timezone"] = m.S.Timezone
		plan["speedLimitMbps"] = integer(content, "speedlimit")
		plan["deviceLimit"] = integer(content, "iplimit")
		plan["groups"] = m.group("0")
		if group := text(content, "node_group"); group != "" && group != "0" {
			plan["groups"] = m.group("0") + "," + m.group(group)
		}
		plan["enabled"] = false
		plan["reviewRequired"] = true
		m.warning("SSPanel 商品 %s 已保留全部商品内容和限制，核对不同分支的套餐开通规则后再开售", name)
		if strings.TrimSpace(name) == "" {
			m.fail("商品 %s 缺少名称", text(row, "id"))
		}
		m.doc("commerce", "plans", m.id("plan", text(row, "id")), plan)
	}
}

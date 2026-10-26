package migration

import (
	"encoding/json"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strconv"
	"strings"
	"time"
)

func (m *mapper) user(row Row, table, loginHash string, account p.Record, balance int64) {
	id := text(row, "id")
	if id == "" || integer(row, "id") <= 0 {
		m.fail("%s 用户标识无效：%q", table, id)
		return
	}
	email := text(row, "email")
	for _, field := range []string{"transfer_enable", "u", "d"} {
		if value, e := strconv.ParseInt(text(row, field), 10, 64); e != nil || value < 0 {
			m.fail("用户 %s 的关键流量字段 %s 缺失或无效", email, field)
		}
	}
	balanceField := "balance"
	if m.S.Source == "sspanel" {
		balanceField = "money"
	}
	if row[balanceField] == nil {
		m.fail("用户 %s 缺少余额字段 %s", email, balanceField)
	}
	algorithm := text(row, "password_algo")
	salt := text(row, "password_salt")
	if algorithm == "" && m.Options.PasswordMethod != "auto" {
		algorithm = m.Options.PasswordMethod
		salt = m.Options.PasswordSalt
	}
	if strings.HasPrefix(loginHash, "$2") {
		algorithm = "bcrypt"
	} else if strings.HasPrefix(loginHash, "$argon2id$") {
		algorithm = "argon2id"
	} else if strings.HasPrefix(loginHash, "$argon2i$") {
		algorithm = "argon2i"
	}
	if algorithm == "sha256" && m.S.Source == "sspanel" {
		algorithm = "sha256salt"
		salt = m.Options.PasswordSalt
	}
	if algorithm == "sha3" {
		algorithm = "sha3salt"
		salt = m.Options.PasswordSalt
	}
	if algorithm == "" || algorithm == "auto" {
		m.fail("用户 %s 的密码算法无法识别，请填写源面板密码算法和盐", email)
	}
	role, status := "member", "active"
	if truth(row, "is_admin") {
		role = "admin"
	}
	if truth(row, "banned") || truth(row, "is_banned") {
		status = "disabled"
	}
	name := text(row, "user_name")
	if name == "" {
		name = email
	}
	created := m.created(row, "created_at", "reg_date")
	secret := ""
	if truth(row, "ga_enable") {
		secret = text(row, "ga_token")
		if secret == "" {
			m.fail("用户 %s 开启了动态验证码但缺少密钥", email)
		}
	}
	for _, field := range []string{"totp_secret", "two_factor_secret"} {
		if value := text(row, field); value != "" {
			secret = value
		}
	}
	if truth(row, "two_factor_enabled") && secret == "" {
		m.fail("用户 %s 的动态验证码配置无法识别", email)
	}
	user := p.ImportUser{ID: m.id("user", id), Email: email, Name: name, Password: loginHash, PasswordAlgorithm: algorithm, PasswordSalt: salt, MFASecret: secret, Role: role, Status: status, CreatedAt: created}
	raw, _ := json.Marshal(user)
	m.Plan.Rows = append(m.Plan.Rows, p.ImportRow{Kind: "user", ID: user.ID, Data: raw})
	m.Plan.Mapped["core/users"]++
	account["userId"] = user.ID
	account["numericId"] = integer(row, "id")
	account["source"] = m.ref(row, table)
	account["status"] = "active"
	if status == "disabled" || truth(row, "is_shadow_banned") || (row["enable"] != nil && !truth(row, "enable")) {
		account["status"] = "suspended"
	}
	account["credential"] = text(row, "uuid")
	if text(row, "uuid") == "" {
		m.fail("用户 %s 缺少 UUID，不能保持原连接凭据", email)
	}
	account["cycleStart"] = m.S.ExportedAt
	account["cycleDays"] = 0
	account["unlimited"] = false
	account["createdAt"] = created
	account["sourceUploadBytes"] = integer(row, "u")
	account["sourceDownloadBytes"] = integer(row, "d")
	m.doc("access", "accounts", m.id("account", id), account)
	m.doc("access", "usage", user.ID+"-"+m.S.ExportedAt, p.Record{"bytes": integer(row, "u") + integer(row, "d")})
	m.doc("commerce", "ledger", m.id("opening-balance", id), p.Record{"userId": user.ID, "amount": balance, "reason": "迁移期初余额", "createdAt": m.S.ExportedAt, "source": m.ref(row, table)})
}
func (m *mapper) v2Users() {
	table, rows := m.table("v2_user")
	if rows == nil {
		m.fail("缺少 v2_user 表")
		return
	}
	for _, row := range rows {
		m.Users[text(row, "id")] = true
	}
	priorBuyers := map[string]bool{}
	for _, order := range m.S.Rows("v2_order") {
		if status := integer(order, "status"); status != 0 && status != 2 {
			priorBuyers[text(order, "user_id")] = true
		}
	}
	for _, row := range rows {
		group := m.group(text(row, "group_id"))
		account := p.Record{"quotaGiB": number(row, "transfer_enable") / 1073741824, "groups": group, "expiresAt": m.timeValue(row, "expired_at"), "speedLimitMbps": integer(row, "speed_limit"), "deviceLimit": integer(row, "device_limit"), "planSourceId": text(row, "plan_id"), "planFamilyId": m.id("plan-family", text(row, "plan_id")), "resetPolicy": m.v2ResetPolicy(row)}
		if row["expired_at"] != nil && text(row, "expired_at") == "0" {
			account["expiresAt"] = time.Unix(0, 0).UTC().Format(time.RFC3339)
		}
		m.user(row, table, text(row, "password"), account, m.money(row, "balance", false))
		id := m.id("user", text(row, "id"))
		if token := text(row, "token"); token != "" {
			m.doc("subscriptions", "links", m.id("subscription", text(row, "id")), p.Record{"userId": id, "token": token, "createdAt": m.created(row, "created_at"), "source": m.ref(row, table)})
		}
		if parent := m.userID(row, "invite_user_id"); parent != "" {
			m.doc("commerce", "referrals", id, p.Record{"userId": id, "parentId": parent, "createdAt": m.created(row, "created_at")})
		}
		if amount := m.money(row, "commission_balance", false); amount != 0 {
			m.doc("commerce", "commissions", m.id("opening-commission", text(row, "id")), p.Record{"userId": id, "amount": amount, "status": "available", "createdAt": m.S.ExportedAt, "source": m.ref(row, table)})
		}
		modeValue := text(row, "commission_type")
		if modeValue == "" {
			modeValue = "0"
		}
		mode := map[string]string{"0": "system", "1": "recurring", "2": "firstPurchase"}[modeValue]
		if mode == "" {
			m.fail("用户 %s 的返佣方式无法识别", text(row, "email"))
		}
		discount, commission := number(row, "discount"), number(row, "commission_rate")
		if discount < 0 || discount > 100 || commission < 0 || commission > 100 {
			m.fail("用户 %s 的折扣或返佣比例无效", text(row, "email"))
		}
		m.doc("commerce", "customers", id, p.Record{"userId": id, "discountPercent": discount, "commissionPercent": commission, "commissionMode": mode, "hasPriorPurchase": priorBuyers[text(row, "id")], "source": m.ref(row, table)})
	}
}
func (m *mapper) ssUsers() {
	table, rows := m.table("user")
	if rows == nil {
		m.fail("缺少 SSPanel user 表")
		return
	}
	for _, row := range rows {
		m.Users[text(row, "id")] = true
	}
	for _, row := range rows {
		group := m.group("0")
		if value := text(row, "node_group"); value != "" && value != "0" {
			group += "," + m.group(value)
		}
		account := p.Record{"quotaGiB": number(row, "transfer_enable") / 1073741824, "groups": group, "allNodeGroups": truth(row, "is_admin"), "accessLevel": integer(row, "class"), "levelExpiresAt": m.timeValue(row, "class_expire"), "proxyPassword": text(row, "passwd"), "speedLimitMbps": integer(row, "node_speedlimit"), "deviceLimit": integer(row, "node_iplimit"), "expiresAt": m.timeValue(row, "expire_in"), "resetPolicy": m.ssResetPolicy(row)}
		m.user(row, table, text(row, "pass"), account, m.money(row, "money", true))
		if parent := m.userID(row, "ref_by"); parent != "" {
			id := m.id("user", text(row, "id"))
			m.doc("commerce", "referrals", id, p.Record{"userId": id, "parentId": parent, "createdAt": m.created(row, "reg_date")})
		}
	}
	links, rows := m.table("link")
	for _, row := range rows {
		user := m.userID(row, "userid")
		if user == "" {
			continue
		}
		token := text(row, "token")
		if token == "" {
			m.fail("SSPanel 订阅 %s 缺少令牌", text(row, "id"))
			continue
		}
		m.doc("subscriptions", "links", m.id("subscription", text(row, "id")), p.Record{"userId": user, "token": token, "createdAt": m.S.ExportedAt, "source": m.ref(row, links)})
	}
}
func (m *mapper) v2ResetPolicy(user Row) p.Record {
	planID := text(user, "plan_id")
	if planID == "" || planID == "0" {
		return p.Record{"type": "never"}
	}
	if m.timeValue(user, "expired_at") == "" {
		return p.Record{"type": "never"}
	}
	var plan Row
	for _, row := range m.S.Rows("v2_plan") {
		if text(row, "id") == planID {
			plan = row
			break
		}
	}
	if plan == nil {
		m.fail("用户 %s 的套餐 %s 不存在", text(user, "id"), planID)
		return p.Record{"type": "never"}
	}
	return m.resetPolicy(plan, user)
}
func (m *mapper) resetPolicy(plan, user Row) p.Record {
	methodText := text(plan, "reset_traffic_method")
	if methodText == "" {
		methodText = m.Options.SystemResetMethod
	}
	if methodText == "" || methodText == "auto" {
		m.fail("套餐 %s 跟随系统流量重置设置，请在任务中填写源面板的系统重置方式", text(plan, "id"))
		return p.Record{"type": "never"}
	}
	method := integer(Row{"method": methodText}, "method")
	if methodText != "0" && method == 0 {
		m.fail("流量重置方式无效：%s", methodText)
	}
	kind := map[int64]string{0: "monthlyDay", 1: "monthlyAnniversary", 2: "never", 3: "yearlyDay", 4: "yearlyAnniversary"}[method]
	if kind == "" {
		m.fail("未知流量重置方式：%d", method)
		kind = "never"
	}
	anchor := m.timeValue(user, "expired_at")
	policy := p.Record{"type": kind, "timezone": m.S.Timezone, "anchorAt": anchor, "nextAt": m.timeValue(user, "next_reset_at"), "day": 1, "month": 1}
	policy["anchorTime"] = m.S.Source == "xboard" && strings.HasSuffix(kind, "Anniversary")
	if value, e := time.Parse(time.RFC3339, anchor); e == nil {
		zone, _ := time.LoadLocation(m.S.Timezone)
		value = value.In(zone)
		if strings.HasSuffix(kind, "Anniversary") {
			policy["day"] = value.Day()
			policy["month"] = int(value.Month())
			if m.S.Source == "xboard" {
				policy["hour"] = value.Hour()
				policy["minute"] = value.Minute()
				policy["second"] = value.Second()
			}
		}
	}
	if m.S.Source == "v2board" {
		policy["skipMissingYearDay"] = true
	}
	return policy
}
func (m *mapper) ssResetPolicy(user Row) p.Record {
	day := integer(user, "auto_reset_day")
	if day == 0 {
		return p.Record{"type": "never"}
	}
	if day < 1 || day > 31 {
		m.fail("用户 %s 重置日无效", text(user, "id"))
	}
	return p.Record{"type": "monthlyDay", "day": day, "timezone": m.S.Timezone, "quotaGiB": number(user, "auto_reset_bandwidth"), "onlyWhileLevelActive": true}
}

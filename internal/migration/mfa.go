package migration

import "strings"

func (m *mapper) ssMFA() {
	table, rows := m.table("mfa_devices")
	other := map[string]bool{}
	for _, row := range rows {
		user := text(row, "userid")
		if !m.Users[user] {
			continue
		}
		if strings.EqualFold(text(row, "type"), "totp") {
			secret := text(object(row, "body"), "token")
			if secret == "" {
				m.fail("%s 用户 %s 的动态验证码设备缺少密钥", table, user)
				continue
			}
			if prior := m.MFA[user]; prior != "" && prior != secret {
				m.fail("用户 %s 有多个不同的动态验证码密钥，需要先确认有效设备", user)
			}
			m.MFA[user] = secret
		} else {
			other[user] = true
		}
	}
	for user := range other {
		if m.MFA[user] == "" {
			m.fail("用户 %s 仅有暂不支持的多因素认证设备，不能静默关闭验证；原设备记录已保留", user)
		} else {
			m.warning("用户 %s 的动态验证码已保留，其他多因素认证设备仅归档", user)
		}
	}
}

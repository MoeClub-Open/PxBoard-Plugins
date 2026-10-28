package migration

import (
	"strings"

	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (m *mapper) v2Templates() {
	_, rows := m.table("v2_subscribe_templates")
	if len(rows) == 0 {
		m.warning("快照未包含订阅模板表，将使用内置模板；原站文件模板可另行导入。")
		return
	}
	policy := p.Record{"clashTemplateId": "builtin-clash", "singboxTemplateId": "builtin-sing-box", "profileTitle": "", "homepage": "", "updateIntervalHours": 6}
	for _, format := range []string{"clash", "sing-box"} {
		m.doc("subscriptions", "templates", "builtin-"+format, p.Record{"name": format + " 默认分流", "format": format, "content": sc.Default(format), "enabled": true, "createdAt": m.S.ExportedAt})
	}
	preferred := map[string]string{}
	for _, row := range rows {
		name := strings.ToLower(text(row, "name"))
		if name != "clash" && name != "clashmeta" && name != "singbox" && name != "sing-box" {
			m.warning("订阅模板 %s 仅归档；当前输出 Mihomo、sing-box 和通用节点链接", name)
			continue
		}
		format, content, err := sc.Xboard(name, text(row, "content"))
		valid := err == nil
		validationError := ""
		if err != nil {
			validationError = err.Error()
			m.warning("订阅模板 %s 已保留为停用草稿：%v；未用于默认订阅", name, err)
			if content == "" {
				content = text(row, "content")
			}
		}
		id := m.S.SourceID + ":" + name
		m.doc("subscriptions", "templates", id, p.Record{"name": m.S.Source + " / " + name, "format": format, "content": content, "originalContent": row["content"], "sourceId": m.S.SourceID, "enabled": valid, "validationError": validationError, "createdAt": m.S.ExportedAt})
		if valid && (preferred[format] == "" || name == "clashmeta") {
			preferred[format] = id
		}
	}
	for format, id := range preferred {
		key := "clashTemplateId"
		if format == "sing-box" {
			key = "singboxTemplateId"
		}
		policy[key] = id
	}
	for _, row := range m.S.Rows("v2_settings") {
		if text(row, "name") == "app_url" {
			policy["homepage"] = text(row, "value")
		}
	}
	m.doc("subscriptions", "settings", "templates", policy)
	m.warning("校验通过的原订阅模板会设为默认，其余格式使用内置模板；原始内容保留供核对，客户端需使用当前 Mihomo 或 sing-box。")
}

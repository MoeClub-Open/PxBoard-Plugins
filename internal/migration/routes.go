package migration

import (
	"encoding/json"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (m *mapper) v2Routes() {
	settings := map[string]string{}
	for _, row := range m.S.Rows("v2_settings") {
		settings[text(row, "name")] = strings.TrimSpace(text(row, "value"))
	}
	admin := settings["secure_path"]
	if admin == "" {
		admin = settings["frontend_admin_path"]
	}
	if admin != "" {
		admin = "/" + strings.Trim(admin, "/")
		raw, _ := json.Marshal(admin)
		m.Plan.Rows = append(m.Plan.Rows, p.ImportRow{Kind: "setting", ID: "adminPath", Data: raw})
		m.Plan.Mapped["settings/adminPath"]++
	} else {
		m.warning("源站后台入口未明确设置，请在迁移后自行设置。")
	}
	path := settings["subscribe_path"]
	if path == "" {
		path = "s"
	}
	base := settings["subscribe_url"]
	if base == "" {
		base = settings["app_url"]
	}
	route := p.Record{"enabled": true, "path": "/" + strings.Trim(path, "/"), "baseUrls": base}
	if _, err := p.PublicRoutePaths(route); err != nil {
		m.fail("旧订阅地址无法自动迁移：%v", err)
		return
	}
	m.doc("subscriptions", "public_routes", "primary", route)
	m.warning("请将原订阅域名指向新面板，并检查反向代理允许旧订阅路径。")
}

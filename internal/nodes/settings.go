package nodes

import (
	"fmt"
	"net/http"
	"slices"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var statusOptions = []p.Option{{Label: "地区", Value: "region"}, {Label: "协议", Value: "protocol"}, {Label: "在线状态", Value: "status"}, {Label: "流量倍率", Value: "trafficRate"}, {Label: "最近心跳", Value: "lastSeen"}, {Label: "CPU 使用率", Value: "cpuPercent"}, {Label: "内存使用率", Value: "memoryPercent"}, {Label: "磁盘使用率", Value: "diskPercent"}, {Label: "交换空间使用率", Value: "swapPercent"}, {Label: "在线账户数", Value: "onlineAccounts"}}
var defaultStatusFields = []string{"region", "protocol", "status", "trafficRate", "lastSeen"}

func (a *app) settingsAPI() {
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /status/display", a.statusDisplay)
	a.View("settings", a.settingsView)
	a.Mux.HandleFunc("GET /_capabilities", func(w http.ResponseWriter, r *http.Request) {
		if !p.Service(r, "core") {
			p.Fail(w, 403, "仅核心可读取功能状态")
			return
		}
		config, e := a.config()
		if !c.Err(w, e) {
			p.JSON(w, p.Record{"portal.status": config["userStatusEnabled"]})
		}
	})
}

func (a *app) config() (p.Record, error) {
	rows, e := a.C.List("settings")
	if e != nil {
		return nil, e
	}
	config := p.Record{"id": "settings", "userStatusEnabled": true, "userStatusFields": defaultStatusFields}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			if enabled, exists := row["userStatusEnabled"]; exists {
				config["userStatusEnabled"] = enabled
			}
			if fields, exists := row["userStatusFields"]; exists {
				config["userStatusFields"] = fields
			}
			break
		}
	}
	return config, nil
}

func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, []p.Record{config})
		return
	}
	var b struct {
		UserStatusEnabled *bool     `json:"userStatusEnabled"`
		UserStatusFields  *[]string `json:"userStatusFields"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if b.UserStatusEnabled != nil {
		config["userStatusEnabled"] = *b.UserStatusEnabled
	}
	if b.UserStatusFields != nil {
		for _, field := range *b.UserStatusFields {
			valid := false
			for _, option := range statusOptions {
				if option.Value == field {
					valid = true
					break
				}
			}
			if !valid {
				p.Fail(w, 400, "节点状态展示项无效")
				return
			}
		}
		config["userStatusFields"] = *b.UserStatusFields
	}
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, config)
	}
}

func (a *app) settingsView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可修改节点设置")
	}
	field := p.Field{Key: "userStatusEnabled", Label: "向用户展示节点状态", Type: "boolean", Default: true}
	fields := p.Field{Key: "userStatusFields", Label: "用户可见的状态项目", Type: "multiselect", Options: statusOptions, Default: defaultStatusFields}
	return p.View{Title: "节点设置", Resources: []p.Resource{{Key: "settings", Title: "用户前台", Endpoint: "/settings", Singleton: true, Editable: true, Columns: []p.Field{field, fields}, Fields: []p.Field{field, fields}}}}, nil
}

func selectedStatusFields(config p.Record) []string {
	result := []string{}
	switch fields := config["userStatusFields"].(type) {
	case []string:
		result = fields
	case []any:
		for _, v := range fields {
			if s, ok := v.(string); ok {
				result = append(result, s)
			}
		}
	}
	return result
}

func (a *app) statusDisplay(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) || !a.requireUserStatus(w, r) {
		return
	}
	config, err := a.config()
	if !c.Err(w, err) {
		p.JSON(w, p.Record{"fields": selectedStatusFields(config)})
	}
}

func filterUserStatus(row p.Record, fields []string) p.Record {
	out := p.Record{"id": row["id"], "name": row["name"]}
	for _, key := range fields {
		if value, ok := row[key]; ok {
			out[key] = value
		}
	}
	if slices.Contains(fields, "status") {
		out["statusLabel"] = row["statusLabel"]
	}
	return out
}

func (a *app) requireUserStatus(w http.ResponseWriter, r *http.Request) bool {
	if p.Admin(r) {
		return true
	}
	config, e := a.config()
	if c.Err(w, e) {
		return false
	}
	if !p.Bool(config, "userStatusEnabled") {
		p.Fail(w, 404, "节点状态暂未开放")
		return false
	}
	return true
}

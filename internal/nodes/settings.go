package nodes

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) settingsAPI() {
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
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
	config := p.Record{"id": "settings", "userStatusEnabled": true}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			if enabled, exists := row["userStatusEnabled"]; exists {
				config["userStatusEnabled"] = enabled
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
		UserStatusEnabled *bool `json:"userStatusEnabled"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if b.UserStatusEnabled != nil {
		config["userStatusEnabled"] = *b.UserStatusEnabled
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
	return p.View{Title: "节点设置", Resources: []p.Resource{{Key: "settings", Title: "用户前台", Endpoint: "/settings", Singleton: true, Editable: true, Columns: []p.Field{field}, Fields: []p.Field{field}}}}, nil
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

package registration

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) config() (p.Record, error) {
	rows, e := a.C.List("settings")
	if e != nil {
		return nil, e
	}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			return row, nil
		}
	}
	return p.Record{"id": "settings", "enabled": true}, nil
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
	var b struct{ Enabled *bool }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if b.Enabled != nil {
		config["enabled"] = *b.Enabled
	}
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, true)
	}
}

func (a *app) policy(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "oauth", "core") {
		return
	}
	config, e := a.config()
	if !c.Err(w, e) {
		p.JSON(w, p.Record{"enabled": p.Bool(config, "enabled")})
	}
}

func (a *app) capabilities(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "core") {
		return
	}
	config, e := a.config()
	if !c.Err(w, e) {
		p.JSON(w, p.Record{"auth.registration": p.Bool(config, "enabled")})
	}
}

func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权管理账户注册")
	}
	fields := []p.Field{c.Field("enabled", "开放注册", "boolean", false)}
	return p.View{Title: "账户注册", Resources: []p.Resource{{Key: "settings", Title: "注册设置", Endpoint: "/settings", Editable: true, Singleton: true, Fields: fields, Columns: fields}}}, nil
}

package captcha

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"strings"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /_challenge", a.publicConfig)
	a.Mux.HandleFunc("POST /_challenge/verify", a.verify)
	a.View("settings", a.view)
	return a
}
func (a *app) config() (p.Record, error) {
	rows, err := a.C.List("settings")
	if err != nil {
		return nil, err
	}
	config := p.Record{"id": "settings", "enabled": false, "provider": "turnstile", "siteKey": "", "secretKey": "", "login": true, "register": true, "reset": true}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			for k, v := range row {
				config[k] = v
			}
			break
		}
	}
	return config, nil
}
func public(config p.Record) p.Record {
	return p.Record{"enabled": p.Bool(config, "enabled"), "provider": p.Text(config, "provider"), "siteKey": p.Text(config, "siteKey"), "login": p.Bool(config, "login"), "register": p.Bool(config, "register"), "reset": p.Bool(config, "reset")}
}
func (a *app) publicConfig(w http.ResponseWriter, r *http.Request) {
	if !p.Service(r, "core") {
		p.Fail(w, 403, "无权读取验证配置")
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	p.JSON(w, public(config))
}
func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !p.Admin(r) {
		p.Fail(w, 403, "仅管理员可修改人机验证")
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	if r.Method == "GET" {
		out := public(config)
		out["hasSecret"] = p.Text(config, "secretKey") != ""
		p.JSON(w, []p.Record{out})
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	oldProvider := p.Text(config, "provider")
	for _, key := range []string{"provider", "siteKey", "secretKey"} {
		if raw, ok := b[key]; ok {
			value, valid := raw.(string)
			if !valid {
				p.Fail(w, 400, "验证配置格式无效")
				return
			}
			value = strings.TrimSpace(value)
			if key != "secretKey" || value != "" {
				config[key] = value
			}
		}
	}
	if p.Text(config, "provider") != oldProvider && strings.TrimSpace(p.Text(b, "secretKey")) == "" {
		config["secretKey"] = ""
	}
	for _, key := range []string{"enabled", "login", "register", "reset"} {
		if raw, ok := b[key]; ok {
			value, valid := raw.(bool)
			if !valid {
				p.Fail(w, 400, "验证配置格式无效")
				return
			}
			config[key] = value
		}
	}
	if !c.Allowed(p.Text(config, "provider"), "turnstile", "hcaptcha", "recaptcha") {
		p.Fail(w, 400, "请选择验证服务")
		return
	}
	if p.Bool(config, "enabled") && (p.Text(config, "siteKey") == "" || p.Text(config, "secretKey") == "") {
		p.Fail(w, 400, "启用前请填写站点密钥和服务端密钥")
		return
	}
	if !c.Err(w, a.C.Put("settings", config)) {
		p.JSON(w, public(config))
	}
}
func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可修改人机验证")
	}
	fields := []p.Field{
		{Key: "enabled", Label: "启用人机验证", Type: "boolean"},
		{Key: "provider", Label: "验证服务", Type: "select", Required: true, Options: []p.Option{{Label: "Cloudflare Turnstile", Value: "turnstile"}, {Label: "hCaptcha", Value: "hcaptcha"}, {Label: "Google reCAPTCHA v2", Value: "recaptcha"}}},
		{Key: "siteKey", Label: "站点密钥", Type: "text"},
		{Key: "secretKey", Label: "服务端密钥", Type: "password", Hint: "留空保留现有密钥"},
		{Key: "login", Label: "密码登录", Type: "boolean"},
		{Key: "register", Label: "账户注册", Type: "boolean"},
		{Key: "reset", Label: "请求密码重置", Type: "boolean"},
	}
	return p.View{Title: "人机验证", Resources: []p.Resource{{Key: "settings", Title: "验证设置", Endpoint: "/settings", Singleton: true, Editable: true, Fields: fields, Columns: []p.Field{fields[0], fields[1], fields[2], fields[4], fields[5], fields[6]}}}}, nil
}

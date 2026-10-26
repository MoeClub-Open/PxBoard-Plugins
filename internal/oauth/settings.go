package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var kinds = map[string]string{"google": "Google", "github": "GitHub", "linuxdo": "LinuxDO"}

func callbackURL(provider p.Record) string {
	if configured := p.Text(provider, "redirectUri"); configured != "" {
		return configured
	}
	return strings.TrimRight(p.Text(provider, "publicUrl"), "/") + "/api/v1/extensions/oauth/callback/" + p.Text(provider, "id")
}

func (a *app) providers(w http.ResponseWriter, r *http.Request) {
	rows, e := a.C.List("providers")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, row := range rows {
		if p.Bool(row, "enabled") {
			out = append(out, p.Record{"id": row["id"], "name": row["name"], "kind": row["kind"], "allowRegistration": row["allowRegistration"]})
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, out)
}

func sanitizedProvider(row p.Record) p.Record {
	out := p.Record{}
	for key, value := range row {
		if key != "clientSecret" {
			out[key] = value
		}
	}
	out["clientSecretConfigured"] = p.Text(row, "clientSecret") != ""
	out["callbackUrl"] = callbackURL(row)
	return out
}

func (a *app) providerSettings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	if r.Method == "GET" {
		rows, e := a.C.List("providers")
		if c.Err(w, e) {
			return
		}
		for i, row := range rows {
			rows[i] = sanitizedProvider(row)
		}
		p.JSON(w, rows)
		return
	}
	var values p.Record
	if c.Err(w, p.Decode(r, &values)) {
		return
	}
	row := p.Record{"id": p.ID(), "enabled": false, "allowRegistration": false, "createdAt": c.Now()}
	if r.Method == "PATCH" {
		var e error
		row, e = a.Find("providers", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		if kind := p.Text(values, "kind"); kind != "" && kind != p.Text(row, "kind") {
			p.Fail(w, 400, "登录服务类型不能变更，请另建配置")
			return
		}
	}
	for _, key := range []string{"name", "kind", "clientId", "clientSecret", "publicUrl", "redirectUri", "enabled", "allowRegistration"} {
		if value, ok := values[key]; ok {
			if key == "clientSecret" && value == "" {
				continue
			}
			row[key] = value
		}
	}
	if kinds[p.Text(row, "kind")] == "" {
		p.Fail(w, 400, "请选择 Google、GitHub 或 LinuxDO")
		return
	}
	if p.Text(row, "name") == "" {
		row["name"] = kinds[p.Text(row, "kind")]
	}
	publicURL := strings.TrimRight(strings.TrimSpace(p.Text(row, "publicUrl")), "/")
	u, e := url.Parse(publicURL)
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && !(u.Scheme == "http" && c.Allowed(u.Hostname(), "localhost", "127.0.0.1", "::1"))) {
		p.Fail(w, 400, "面板地址需为 HTTPS 根地址，本机开发可使用 HTTP")
		return
	}
	row["publicUrl"], row["updatedAt"] = publicURL, c.Now()
	if redirect := p.Text(row, "redirectUri"); redirect != "" {
		callback, e := url.Parse(redirect)
		if e != nil || callback.Scheme != u.Scheme || !strings.EqualFold(callback.Hostname(), u.Hostname()) || callback.User != nil || callback.RawQuery != "" || callback.Fragment != "" || !strings.HasPrefix(callback.Path, "/") {
			p.Fail(w, 400, "回调地址必须位于同一面板域名下，且不含查询参数")
			return
		}
	}
	if c.Err(w, c.Validate(row, "clientId", "clientSecret")) {
		return
	}
	if !c.Err(w, a.C.Put("providers", row)) {
		p.JSON(w, sanitizedProvider(row))
	}
}

func (a *app) deleteProvider(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("bindings")
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		if p.Text(row, "providerId") == r.PathValue("id") {
			p.Fail(w, 409, "此服务已有登录绑定，请停用配置或先处理绑定")
			return
		}
	}
	if !c.Err(w, a.C.Delete("providers", r.PathValue("id"))) {
		p.JSON(w, true)
	}
}

func (a *app) providerView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权管理登录服务")
	}
	f := []p.Field{
		c.Select("kind", "登录服务", "google|Google", "github|GitHub", "linuxdo|LinuxDO"),
		c.Field("name", "显示名称", "text", false),
		c.Field("publicUrl", "面板地址", "text", true),
		c.Field("clientId", "客户端 ID", "text", true),
		c.Field("clientSecret", "客户端密钥", "password", false),
		c.Field("redirectUri", "已登记回调地址（可选）", "text", false),
		c.Field("enabled", "启用", "boolean", false),
		c.Field("allowRegistration", "允许新账号注册", "boolean", false),
	}
	return p.View{Title: "第三方登录", Resources: []p.Resource{{Key: "providers", Title: "登录服务", Endpoint: "/settings/providers", CreateLabel: "添加登录服务", Editable: true, Deletable: true, Fields: f, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("kind", "服务", "text", false), c.Field("enabled", "启用", "boolean", false), c.Field("allowRegistration", "允许注册", "boolean", false), c.Field("callbackUrl", "授权回调地址", "copy", false)}}}}, nil
}

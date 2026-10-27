package notifications

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/mail"
	"net/url"
)

func (a *app) mailConfig(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	if r.Method == "GET" {
		rows, e := a.C.List("mail")
		for _, row := range rows {
			delete(row, "password")
		}
		if !c.Err(w, e) {
			p.JSON(w, rows)
		}
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	rows, e := a.C.List("mail")
	if c.Err(w, e) {
		return
	}
	if len(rows) > 0 {
		old := rows[0]
		for k, v := range b {
			if k == "password" && v == "" {
				continue
			}
			old[k] = v
		}
		b = old
	}
	b["id"] = "smtp"
	if c.Err(w, c.Validate(b, "host", "from", "publicUrl")) {
		return
	}
	if _, e = mail.ParseAddress(p.Text(b, "from")); c.Err(w, e) {
		return
	}
	publicURL, e := url.Parse(p.Text(b, "publicUrl"))
	if e != nil || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" || !c.Allowed(publicURL.Scheme, "http", "https") {
		p.Fail(w, 400, "请填写完整的面板 HTTP 或 HTTPS 地址")
		return
	}
	if !c.Err(w, a.C.Put("mail", b)) {
		p.JSON(w, true)
	}
}
func (a *app) notifications(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("notifications")
	for _, row := range rows {
		delete(row, "body")
	}
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}

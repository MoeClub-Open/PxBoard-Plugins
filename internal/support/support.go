package support

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"sync"
)

type app struct {
	*c.App
	delivery sync.Mutex
}

func New() http.Handler {
	a := &app{App: c.New()}
	for _, key := range []string{"announcements", "articles"} {
		a.CRUD(contentResource(key), func(r *http.Request, b, old p.Record) error {
			c.Defaults(b, old, p.Record{"published": true})
			return c.Validate(b, "title", "content")
		}, func(r *http.Request, row p.Record) bool { return p.Admin(r) || p.Bool(row, "published") })
	}
	a.Mux.HandleFunc("GET /tickets", a.tickets)
	a.Mux.HandleFunc("POST /tickets", a.ticket)
	a.Mux.HandleFunc("POST /tickets/{id}/{action}", a.ticketAction)
	a.Mux.HandleFunc("GET /mail", a.mailConfig)
	a.Mux.HandleFunc("POST /mail", a.mailConfig)
	a.Mux.HandleFunc("PATCH /mail/{id}", a.mailConfig)
	a.Mux.HandleFunc("GET /notifications", a.notifications)
	a.Mux.HandleFunc("POST /notifications/{id}/retry", a.retryNotification)
	a.Mux.HandleFunc("GET /_capabilities", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r, "core") {
			return
		}
		rows, e := a.C.List("mail")
		if !c.Err(w, e) {
			p.JSON(w, map[string]bool{"auth.password-reset": len(rows) > 0 && p.Bool(rows[0], "enabled")})
		}
	})
	a.View("tickets", a.ticketView)
	a.View("content", a.contentView)
	a.View("notifications", a.notificationView)
	a.Events(a.event)
	return a
}
func (a *app) tickets(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, e := a.Filter("tickets", func(row p.Record) bool { return c.OwnList(r, row) })
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) ticket(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if c.Err(w, c.Validate(b, "subject", "content")) {
		return
	}
	row := p.Record{"id": p.ID(), "userId": p.Subject(r).ID, "email": p.Subject(r).Email, "subject": b["subject"], "thread": p.Subject(r).Email + " · " + c.Now() + "\n" + p.Text(b, "content"), "status": "open", "createdAt": c.Now(), "updatedAt": c.Now()}
	if !c.Err(w, a.C.Put("tickets", row)) {
		p.JSON(w, row)
	}
}
func (a *app) ticketAction(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.Find("tickets", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if !c.Own(r, row) {
		p.Fail(w, 403, "无权操作此工单")
		return
	}
	switch r.PathValue("action") {
	case "reply":
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if c.Err(w, c.Validate(b, "message")) {
			return
		}
		row["thread"] = p.Text(row, "thread") + "\n\n" + p.Subject(r).Email + " · " + c.Now() + "\n" + p.Text(b, "message")
		if p.Admin(r) {
			row["status"] = "answered"
			pending, _ := row["pendingNotifications"].([]any)
			row["pendingNotifications"] = append(pending, map[string]any{"userId": row["userId"], "subject": row["subject"], "ticketId": row["id"], "messageId": p.ID()})
		} else {
			row["status"] = "open"
		}
	case "close":
		row["status"] = "closed"
	default:
		p.Fail(w, 404, "操作不存在")
		return
	}
	row["updatedAt"] = c.Now()
	if c.Err(w, a.C.Put("tickets", row)) {
		return
	}
	if p.Admin(r) && r.PathValue("action") == "reply" {
		if err := a.flushTicketNotifications(row); err != nil {
			log.Printf("工单回复已保存，通知等待重试：%v", err)
		}
	}
	delete(row, "pendingNotifications")
	p.JSON(w, row)
}
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

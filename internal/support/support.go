package support

import (
	"crypto/tls"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
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
	rows, e := a.Filter("tickets", func(row p.Record) bool { return c.Own(r, row) })
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
	if p.Admin(r) {
		if c.Err(w, a.C.Emit("ticket.replied", p.Record{"userId": row["userId"], "subject": row["subject"], "ticketId": row["id"]})) {
			return
		}
	}
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
func (a *app) event(e p.Event) error {
	if e.Type == "system.tick" {
		configs, err := a.C.List("mail")
		if err != nil {
			return err
		}
		if len(configs) == 0 || !p.Bool(configs[0], "enabled") {
			return nil
		}
		rows, err := a.C.List("notifications")
		if err != nil {
			return err
		}
		for _, row := range rows {
			if p.Text(row, "status") == "sent" || p.Number(row, "attempts") >= 5 {
				continue
			}
			err = send(configs[0], row)
			row["attempts"] = p.Number(row, "attempts") + 1
			if err != nil {
				row["status"] = "failed"
				row["error"] = err.Error()
			} else {
				row["status"] = "sent"
				row["sentAt"] = c.Now()
				delete(row, "body")
			}
			if err = a.C.Put("notifications", row); err != nil {
				return err
			}
		}
		return nil
	}
	if e.Type == "auth.reset-requested" && e.Source != "core" || e.Type == "order.fulfilled" && e.Source != "commerce" || e.Type == "ticket.replied" && e.Source != "support" {
		return fmt.Errorf("通知事件来源无效")
	}
	users, err := a.C.Users()
	if err != nil {
		return err
	}
	email := ""
	for _, u := range users {
		if u["id"] == e.Data["userId"] {
			email = p.Text(u, "email")
		}
	}
	if email == "" {
		return nil
	}
	subject, body := "", ""
	switch e.Type {
	case "order.fulfilled":
		subject = "订单已开通"
		body = "您的订单已完成：" + p.Text(e.Data, "planName")
	case "ticket.replied":
		subject = "工单有新回复"
		body = "您的工单「" + p.Text(e.Data, "subject") + "」已收到回复，请登录查看。"
	case "auth.reset-requested":
		configs, err := a.C.List("mail")
		if err != nil {
			return err
		}
		if len(configs) == 0 {
			return nil
		}
		subject = "重置登录密码"
		body = "请在 30 分钟内打开此链接重置密码：\n" + strings.TrimSuffix(p.Text(configs[0], "publicUrl"), "/") + "/reset?token=" + p.Text(e.Data, "token")
	default:
		return nil
	}
	return a.C.Put("notifications", p.Record{"id": e.ID, "to": email, "subject": subject, "body": body, "status": "pending", "attempts": 0.0, "createdAt": c.Now()})
}
func send(config, message p.Record) error {
	host := p.Text(config, "host")
	port := int(p.Number(config, "port"))
	if port == 0 {
		port = 587
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	if port == 465 {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	} else {
		conn, err = dialer.Dial("tcp", address)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if port != 465 {
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if p.Text(config, "username") != "" {
		if err = client.Auth(smtp.PlainAuth("", p.Text(config, "username"), p.Text(config, "password"), host)); err != nil {
			return err
		}
	}
	from := p.Text(config, "from")
	to := p.Text(message, "to")
	if strings.ContainsAny(from+to+p.Text(message, "subject"), "\r\n") {
		return fmt.Errorf("邮件头无效")
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", from, to, p.Text(message, "subject"), p.Text(message, "body"))
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

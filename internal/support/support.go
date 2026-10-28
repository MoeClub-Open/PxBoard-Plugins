package support

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{App: c.New()}
	for _, key := range []string{"announcements", "articles"} {
		a.CRUD(contentResource(key), func(r *http.Request, b, old p.Record) error {
			c.Defaults(b, old, p.Record{"published": true})
			return c.Validate(b, "title", "content")
		}, func(r *http.Request, row p.Record) bool { return p.Admin(r) || p.Bool(row, "published") })
	}
	a.Mux.HandleFunc("POST /attachments", a.upload)
	a.Mux.HandleFunc("GET /attachments/{id}", a.download)
	a.Mux.HandleFunc("GET /tickets", a.tickets)
	a.Mux.HandleFunc("POST /tickets", a.ticket)
	a.Mux.HandleFunc("POST /tickets/{id}/{action}", a.ticketAction)
	a.View("tickets", a.ticketView)
	a.View("content", a.contentView)
	return a
}
func (a *app) tickets(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	result, e := a.ReadRecords("tickets", r, c.OwnerFilters(r), "subject", "email", "status")
	if !c.Err(w, e) {
		c.ListResponse(w, r, result)
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
	ids, err := a.validateAttachments(r, p.Text(b, "content"), nil)
	if c.Err(w, err) {
		return
	}
	row := p.Record{"id": p.ID(), "userId": p.Subject(r).ID, "email": p.Subject(r).Email, "subject": b["subject"], "messages": []p.Record{message(r, p.Text(b, "content"), ids)}, "status": "open", "createdAt": c.Now(), "updatedAt": c.Now()}
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
	var events []p.Emission
	operation := p.ID()
	switch r.PathValue("action") {
	case "reply":
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if c.Err(w, c.Validate(b, "message")) {
			return
		}
		ids, err := a.validateAttachments(r, p.Text(b, "message"), row)
		if c.Err(w, err) {
			return
		}
		messages, _ := row["messages"].([]any)
		row["messages"] = append(messages, message(r, p.Text(b, "message"), ids))
		if p.Admin(r) {
			row["status"] = "answered"
			events = append(events, p.Emission{Type: "ticket.replied", Data: p.Record{"userId": row["userId"], "subject": row["subject"], "ticketId": row["id"], "messageId": operation}})
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
	if c.Err(w, a.C.Commit("ticket:"+operation, []p.Write{{Collection: "tickets", Record: row}}, events...)) {
		return
	}
	p.JSON(w, row)
}

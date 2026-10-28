package notifications

import (
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) loadTemplate(id string) (mailTemplate, error) {
	item, ok := defaultTemplate(id)
	if !ok {
		return item, &p.APIError{Status: 404, Message: "邮件模板不存在"}
	}
	rows, err := a.QueryAll("templates", p.Record{"field": "id", "value": id})
	if err != nil || len(rows) == 0 {
		return item, err
	}
	item.Subject, item.Text, item.HTML = p.Text(rows[0], "subject"), p.Text(rows[0], "text"), p.Text(rows[0], "html")
	item.Customized, item.UpdatedAt = true, p.Text(rows[0], "updatedAt")
	return item, nil
}

func (a *app) templates(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows := []p.Record{}
	for _, builtin := range templateDefaults {
		item, err := a.loadTemplate(builtin.ID)
		if c.Err(w, err) {
			return
		}
		rows = append(rows, p.Record{"id": item.ID, "name": item.Name, "subject": item.Subject, "customized": item.Customized, "updatedAt": item.UpdatedAt})
	}
	p.JSON(w, rows)
}

func (a *app) templateAction(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	id := r.PathValue("id")
	item, err := a.loadTemplate(id)
	if failure, ok := err.(*p.APIError); ok {
		p.Fail(w, failure.Status, failure.Message)
		return
	}
	if c.Err(w, err) {
		return
	}
	if r.Method == "GET" {
		p.JSON(w, struct {
			mailTemplate
			Variables []map[string]string `json:"variables"`
		}{item, templateVariables(id)})
		return
	}
	if r.Method == "POST" && r.PathValue("action") != "reset" && r.PathValue("action") != "preview" {
		p.Fail(w, 404, "操作不存在")
		return
	}
	if r.PathValue("action") == "reset" {
		if !c.Err(w, a.C.Delete("templates", id)) {
			p.JSON(w, true)
		}
		return
	}
	var body struct{ Subject, Text, HTML string }
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	item.Subject, item.Text, item.HTML = body.Subject, body.Text, body.HTML
	data := sampleData()
	data.SiteName = a.SiteName()
	if id != "general" {
		data.Subject, data.Message = "", ""
	}
	if id == "password-reset" {
		data.ActionURL, data.ActionLabel = data.ResetURL, "重置密码"
	} else if id == "order-fulfilled" {
		data.ActionURL, data.ActionLabel = data.PublicURL+"/user/orders", "查看订单"
	} else if id == "ticket-replied" {
		data.ActionURL, data.ActionLabel = data.PublicURL+"/user/support", "查看工单"
	}
	rendered, err := renderMail(item, data)
	if c.Err(w, err) {
		return
	}
	if r.PathValue("action") == "preview" {
		p.JSON(w, rendered)
		return
	}
	if !c.Err(w, a.C.Put("templates", p.Record{"id": id, "subject": item.Subject, "text": item.Text, "html": item.HTML, "updatedAt": c.Now()})) {
		p.JSON(w, true)
	}
}

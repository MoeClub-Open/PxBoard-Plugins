package notifications

import (
	"fmt"
	"strings"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) composeEvent(event p.Event, user p.Record, override ...mailTemplate) (p.Record, error) {
	configs, err := a.C.List("mail")
	if err != nil {
		return nil, err
	}
	base := ""
	if len(configs) > 0 {
		base = strings.TrimRight(p.Text(configs[0], "publicUrl"), "/")
	}
	data := templateData{SiteName: a.SiteName(), UserName: p.Text(user, "name"), Email: p.Text(user, "email"), PublicURL: base, ActionLabel: "查看详情"}
	if data.UserName == "" {
		data.UserName = data.Email
	}
	if base != "" {
		data.ActionURL = base + "/user"
	}
	id, expiresAt := "", ""
	switch event.Type {
	case "order.fulfilled":
		id = "order-fulfilled"
		data.OrderID, data.OrderNumber, data.PlanName = p.Text(event.Data, "orderId"), p.Text(event.Data, "orderNumber"), p.Text(event.Data, "planName")
		if data.OrderNumber == "" {
			data.OrderNumber = data.OrderID
		}
		if base != "" {
			data.ActionURL, data.ActionLabel = base+"/user/orders", "查看订单"
		}
	case "ticket.replied":
		id = "ticket-replied"
		data.TicketID, data.TicketSubject = p.Text(event.Data, "ticketId"), p.Text(event.Data, "subject")
		if base != "" {
			data.ActionURL, data.ActionLabel = base+"/user/support", "查看工单"
		}
	case "auth.reset-requested":
		if len(configs) == 0 || !p.Bool(configs[0], "enabled") {
			return nil, nil
		}
		created, err := time.Parse(time.RFC3339, event.CreatedAt)
		if err != nil {
			return nil, err
		}
		id, expiresAt = "password-reset", created.Add(30*time.Minute).UTC().Format(time.RFC3339)
		data.ResetURL = base + "/reset?token=" + p.Text(event.Data, "token")
		data.ActionURL, data.ActionLabel, data.ExpiryMinutes = data.ResetURL, "重置密码", 30
	case "access.reminder":
		id = "access-expiry"
		if p.Text(event.Data, "kind") == "traffic" {
			id = "access-traffic"
		}
		data.ExpiresAt = p.Text(event.Data, "expiresAt")
		if expiry, err := time.Parse(time.RFC3339, data.ExpiresAt); err == nil {
			data.ExpiresAt = expiry.Format("2006-01-02 15:04 UTC")
		}
		data.UsedGiB, data.QuotaGiB = p.Number(event.Data, "usedGiB"), p.Number(event.Data, "quotaGiB")
		expiresAt = time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	case "notification.requested":
		id = "general"
		data.Subject, data.Message = strings.TrimSpace(p.Text(event.Data, "subject")), p.Text(event.Data, "body")
		if data.Subject == "" || data.Message == "" {
			return nil, fmt.Errorf("通知缺少主题或内容")
		}
	default:
		return nil, nil
	}
	var item mailTemplate
	if len(override) > 0 {
		item = override[0]
	} else {
		item, err = a.loadTemplate(id)
	}
	if err != nil {
		return nil, err
	}
	rendered, err := renderMail(item, data)
	if err != nil {
		return nil, err
	}
	return p.Record{"to": data.Email, "subject": rendered.Subject, "body": rendered.Text, "html": rendered.HTML, "templateId": id, "expiresAt": expiresAt}, nil
}

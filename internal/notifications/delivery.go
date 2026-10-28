package notifications

import (
	"fmt"
	"log"
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) event(e p.Event) error {
	if e.Type == "system.tick" {
		if e.Source != "core" {
			return fmt.Errorf("通知事件来源无效")
		}
		a.startDelivery()
		return nil
	}
	if e.Type == "auth.reset-requested" && e.Source != "core" || e.Type == "order.fulfilled" && e.Source != "commerce" || e.Type == "ticket.replied" && e.Source != "support" {
		return fmt.Errorf("通知事件来源无效")
	}
	id := e.ID
	if orderID := p.Text(e.Data, "orderId"); e.Type == "order.fulfilled" && orderID != "" {
		id = "order-" + orderID
	}
	if messageID := p.Text(e.Data, "messageId"); e.Type == "ticket.replied" && messageID != "" {
		id = "reply-" + messageID
	}
	known, err := a.C.Query(p.Query{Collection: "notifications", Limit: 1, Filters: []p.Filter{{Field: "id", Value: id}}})
	if err != nil {
		return err
	}
	if len(known.Items) > 0 {
		return nil
	}

	users, err := a.C.UsersByID(p.Text(e.Data, "userId"))
	if err != nil {
		return err
	}
	if len(users) == 0 || p.Text(users[0], "email") == "" {
		return nil
	}
	row, err := a.composeEvent(e, users[0])
	if err != nil || row == nil {
		return err
	}
	row["id"], row["status"], row["attempts"], row["createdAt"] = id, "pending", 0, e.CreatedAt
	if err = a.C.Put("notifications", row); err != nil {
		return err
	}

	a.startDelivery()
	return nil
}

func (a *app) startDelivery() {
	if !a.delivery.TryLock() {
		return
	}
	go func() {
		defer a.delivery.Unlock()
		if err := a.deliver(); err != nil {
			log.Printf("邮件队列处理失败：%v", err)
		}
	}()
}

func (a *app) deliver() error {
	configs, err := a.C.List("mail")
	if err != nil || len(configs) == 0 || !p.Bool(configs[0], "enabled") {
		return err
	}
	rows, err := a.QueryAll("notifications", p.Record{"field": "status", "op": "in", "value": []string{"pending", "failed"}})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if p.Number(row, "attempts") >= 5 || p.Text(row, "nextAttemptAt") > c.Now() {
			continue
		}
		if expiry := p.Text(row, "expiresAt"); expiry != "" && expiry <= c.Now() {
			row["status"] = "expired"
			delete(row, "body")
			delete(row, "html")
		} else {
			err = send(configs[0], row)
			attempts := int(p.Number(row, "attempts")) + 1
			row["attempts"] = attempts
			if err != nil {
				row["status"], row["error"] = "failed", err.Error()
				row["nextAttemptAt"] = time.Now().Add(time.Duration(1<<min(attempts-1, 4)) * time.Minute).UTC().Format(time.RFC3339)
			} else {
				row["status"], row["sentAt"] = "sent", c.Now()
				delete(row, "body")
				delete(row, "html")
				delete(row, "error")
				delete(row, "nextAttemptAt")
			}
		}
		if err = a.C.Put("notifications", row); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) retryNotification(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	if !a.delivery.TryLock() {
		p.Fail(w, 409, "邮件队列正在投递，请稍后重试")
		return
	}
	row, err := a.Find("notifications", r.PathValue("id"))
	if err == nil && p.Text(row, "status") != "failed" {
		err = fmt.Errorf("只有投递失败的邮件可以重试")
	}
	if err == nil {
		row["status"], row["attempts"] = "pending", 0
		delete(row, "nextAttemptAt")
		delete(row, "error")
		err = a.C.Put("notifications", row)
	}
	a.delivery.Unlock()
	if !c.Err(w, err) {
		a.startDelivery()
		p.JSON(w, true)
	}
}

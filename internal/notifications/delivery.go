package notifications

import (
	"fmt"
	"log"
	"net/http"
	"strings"
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
	rows, err := a.C.List("notifications")
	if err != nil {
		return err
	}
	for _, row := range rows {
		if p.Text(row, "id") == id {
			return nil
		}
	}
	users, err := a.C.Users()
	if err != nil {
		return err
	}
	email := ""
	for _, u := range users {
		if u["id"] == e.Data["userId"] {
			email = p.Text(u, "email")
			break
		}
	}
	if email == "" {
		return nil
	}
	subject, body, expiresAt := "", "", ""
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
		if len(configs) == 0 || !p.Bool(configs[0], "enabled") {
			return nil
		}
		createdAt, err := time.Parse(time.RFC3339, e.CreatedAt)
		if err != nil {
			return err
		}
		expiresAt = createdAt.Add(30 * time.Minute).UTC().Format(time.RFC3339)
		subject = "重置登录密码"
		body = "请在 30 分钟内打开此链接重置密码：\n" + strings.TrimSuffix(p.Text(configs[0], "publicUrl"), "/") + "/reset?token=" + p.Text(e.Data, "token")
	case "notification.requested":
		subject, body = strings.TrimSpace(p.Text(e.Data, "subject")), p.Text(e.Data, "body")
		if subject == "" || body == "" {
			return fmt.Errorf("通知缺少主题或内容")
		}
	default:
		return nil
	}
	if err = a.C.Put("notifications", p.Record{"id": id, "to": email, "subject": subject, "body": body, "status": "pending", "attempts": 0.0, "createdAt": e.CreatedAt, "expiresAt": expiresAt}); err != nil {
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
	rows, err := a.C.List("notifications")
	if err != nil {
		return err
	}
	for _, row := range rows {
		status := p.Text(row, "status")
		if status == "sent" || status == "expired" || p.Number(row, "attempts") >= 5 || p.Text(row, "nextAttemptAt") > c.Now() {
			continue
		}
		if expiry := p.Text(row, "expiresAt"); expiry != "" && expiry <= c.Now() {
			row["status"] = "expired"
			delete(row, "body")
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

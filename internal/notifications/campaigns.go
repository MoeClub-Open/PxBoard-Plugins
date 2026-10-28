package notifications

import (
	"fmt"
	"net/mail"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) campaignTargets(row p.Record) ([]p.Record, error) {
	users := []p.Record{}
	if p.Text(row, "audience") == "selected" {
		seen := map[string]bool{}
		for _, email := range strings.FieldsFunc(p.Text(row, "emails"), func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' }) {
			email = strings.ToLower(strings.TrimSpace(email))
			if seen[email] {
				continue
			}
			seen[email] = true
			if _, err := mail.ParseAddress(email); err != nil {
				return nil, fmt.Errorf("邮箱格式无效：%s", email)
			}
			matches, err := a.C.UsersByEmail(email)
			if err != nil {
				return nil, err
			}
			if len(matches) == 0 || p.Text(matches[0], "status") != "active" {
				return nil, fmt.Errorf("邮箱未注册或账户已停用：%s", email)
			}
			users = append(users, matches[0])
		}
	} else {
		for offset := 0; ; {
			page, err := a.C.SearchUsers(p.UserQuery{Search: p.Text(row, "search"), Status: "active", Role: "member", Offset: offset, Limit: 500})
			if err != nil {
				return nil, err
			}
			users = append(users, page.Items...)
			offset += len(page.Items)
			if offset >= page.Total || len(page.Items) == 0 {
				break
			}
		}
	}
	return users, nil
}

func campaignIDs(row p.Record) []string {
	ids := []string{}
	if list, ok := row["recipients"].([]any); ok {
		for _, id := range list {
			if s, ok := id.(string); ok {
				ids = append(ids, s)
			}
		}
	}
	if list, ok := row["recipients"].([]string); ok {
		return list
	}
	return ids
}

func (a *app) campaignMail(row, user p.Record) (p.Record, error) {
	e := p.Event{Type: "notification.requested", Data: p.Record{"subject": row["subject"], "body": row["body"]}}
	templates := []mailTemplate{}
	if raw, ok := row["mailTemplate"].(map[string]any); ok {
		templates = append(templates, mailTemplate{ID: "general", Subject: p.Text(raw, "subject"), Text: p.Text(raw, "text"), HTML: p.Text(raw, "html")})
	}
	mail, err := a.composeEvent(e, user, templates...)
	if err != nil {
		return nil, err
	}
	mail["id"] = "campaign:" + p.Text(row, "id") + ":" + p.Text(user, "id")
	mail["campaignId"], mail["userId"] = row["id"], user["id"]
	mail["status"], mail["attempts"], mail["createdAt"] = "pending", 0, c.Now()
	return mail, nil
}

// Enqueue one bounded batch per campaign. The cursor and messages commit in
// one transaction; a restart cannot lose a recipient or enqueue it twice.
func (a *app) queueCampaigns() error {
	a.campaigns.Lock()
	defer a.campaigns.Unlock()
	rows, err := a.QueryAll("campaigns", p.Record{"field": "status", "value": "running"})
	if err != nil {
		return err
	}
	for _, row := range rows {
		ids := campaignIDs(row)
		cursor := int(p.Number(row, "queued"))
		if cursor >= len(ids) {
			continue
		}
		end := min(cursor+100, len(ids))
		users, err := a.C.UsersByID(ids[cursor:end]...)
		if err != nil {
			return err
		}
		byID := map[string]p.Record{}
		for _, user := range users {
			byID[p.Text(user, "id")] = user
		}
		writes := []p.Write{}
		for _, id := range ids[cursor:end] {
			user := byID[id]
			var mail p.Record
			if user == nil || p.Text(user, "status") != "active" {
				mail = p.Record{"id": "campaign:" + p.Text(row, "id") + ":" + id, "campaignId": row["id"], "userId": id, "status": "skipped", "createdAt": c.Now()}
			} else {
				mail, err = a.campaignMail(row, user)
				if err != nil {
					return err
				}
			}
			writes = append(writes, p.Write{Collection: "notifications", Record: mail})
		}
		row["queued"], row["updatedAt"] = end, c.Now()
		writes = append(writes, p.Write{Collection: "campaigns", Record: row})
		if err = a.C.Commit(fmt.Sprintf("campaign:%s:%d", p.Text(row, "id"), cursor), writes); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) campaignSummary(row p.Record) (p.Record, error) {
	out := p.Record{}
	for k, v := range row {
		if k != "recipients" {
			out[k] = v
		}
	}
	rows, err := a.QueryAll("notifications", p.Record{"field": "campaignId", "value": row["id"]})
	counts := map[string]int{}
	for _, mail := range rows {
		status := p.Text(mail, "status")
		if status == "failed" && p.Number(mail, "attempts") < 5 {
			status = "pending"
		}
		counts[status]++
	}
	if p.Text(row, "status") != "canceled" {
		counts["pending"] += max(0, len(campaignIDs(row))-int(p.Number(row, "queued")))
	}
	for _, key := range []string{"sent", "failed", "pending", "skipped", "canceled"} {
		out[key+"Count"] = counts[key]
	}
	if p.Text(row, "status") == "running" && int(p.Number(row, "queued")) >= len(campaignIDs(row)) && counts["pending"] == 0 {
		out["status"] = "completed"
	}
	return out, err
}

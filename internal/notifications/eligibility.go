package notifications

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) notificationAllowed(row p.Record) (bool, error) {
	status := ""
	if campaign := p.Text(row, "campaignId"); campaign != "" {
		parent, err := a.Find("campaigns", campaign)
		if err != nil {
			return false, err
		}
		switch p.Text(parent, "status") {
		case "canceled":
			status = "canceled"
		case "paused":
			return false, nil
		}
		users, err := a.C.UsersByID(p.Text(row, "userId"))
		if err != nil {
			return false, err
		}
		if len(users) == 0 || p.Text(users[0], "status") != "active" || p.Text(users[0], "email") != p.Text(row, "to") {
			status = "skipped"
		}
	}
	if reminder, ok := row["reminder"].(map[string]any); ok {
		var result struct{ Valid bool }
		if err := a.C.Call("access", "POST", "/internal/reminder-valid", reminder, &result); err != nil {
			return false, err
		}
		if !result.Valid {
			status = "expired"
		}
	}
	if status == "" {
		return true, nil
	}
	row["status"], row["updatedAt"] = status, c.Now()
	delete(row, "body")
	delete(row, "html")
	return false, a.C.Put("notifications", row)
}

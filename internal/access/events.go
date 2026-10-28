package access

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) event(e p.Event) error {
	switch e.Type {
	case "workspace.configured":
		if e.Source != "core" {
			return fmt.Errorf("工作空间事件来源无效")
		}
		mode, user := p.Text(e.Data, "mode"), p.Text(e.Data, "userId")
		if (mode != "personal" && mode != "sharing") || user == "" {
			return nil
		}
		rows, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		number := 0.0
		for _, row := range rows {
			if p.Text(row, "userId") == user {
				return nil
			}
			number = max(number, p.Number(row, "numericId"))
		}
		return a.C.Put("accounts", p.Record{"id": p.ID(), "userId": user, "numericId": number + 1, "credential": uuid(), "status": "active", "quotaGiB": 0, "unlimited": true, "cycleStart": c.Now(), "cycleDays": 0, "expiresAt": "", "groups": "default", "createdAt": c.Now()})
	case "traffic.reported":
		if e.Source != "nodes" {
			return fmt.Errorf("流量来源无效")
		}
		accounts, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		byUser := map[string]p.Record{}
		for _, row := range accounts {
			byUser[p.Text(row, "userId")] = row
		}
		usage, _ := e.Data["usage"].([]any)
		for _, raw := range usage {
			u, _ := raw.(map[string]any)
			user := p.Text(u, "userId")
			row := byUser[user]
			if row == nil || e.CreatedAt < p.Text(row, "cycleStart") {
				continue
			}
			operation := p.Text(e.Data, "nodeId") + "-" + p.Text(e.Data, "reportId") + "-" + user
			if err = a.C.Increment("usage", user+"-"+p.Text(row, "cycleStart"), "bytes", p.Number(u, "bytes")*p.Number(e.Data, "rate"), operation); err != nil {
				return err
			}
		}
		return nil
	case "access.suspend":
		if e.Source != "audit" {
			return fmt.Errorf("审计来源无效")
		}
		rows, err := a.C.List("accounts")
		if err != nil {
			return err
		}
		for _, row := range rows {

			if row["userId"] == e.Data["userId"] {
				row["status"] = "suspended"
				row["suspendReason"] = e.Data["reason"]
				if err = a.C.Put("accounts", row); err != nil {
					return err
				}
			}
		}
	case "system.tick":
		if err := a.resetCycles(); err != nil {
			return err
		}
		return a.sendReminders()
	}
	return nil
}

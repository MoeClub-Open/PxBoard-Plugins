package access

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"time"
)

func (a *app) resetCycles() error {
	rows, err := a.C.List("accounts")
	if err != nil {
		return err
	}
	updates := []p.Record{}
	for _, row := range rows {
		if policy, ok := row["resetPolicy"].(map[string]any); ok {
			if p.Text(policy, "type") == "never" {
				continue
			}
			if p.Text(row, "expiresAt") != "" && p.Text(row, "expiresAt") <= c.Now() {
				continue
			}
			if expiry := p.Text(row, "levelExpiresAt"); p.Bool(policy, "onlyWhileLevelActive") && expiry != "" && expiry <= c.Now() {
				continue
			}
			next, e := time.Parse(time.RFC3339, p.Text(policy, "nextAt"))
			initialized := e == nil
			if e != nil {
				start, e := time.Parse(time.RFC3339, p.Text(row, "cycleStart"))
				if e != nil {
					return e
				}
				next, e = c.NextReset(policy, start)
				if e != nil {
					return e
				}
			}
			if next.IsZero() {
				continue
			}
			if initialized && next.After(time.Now()) {
				continue
			}
			for !next.After(time.Now()) {
				row["cycleStart"] = next.UTC().Format(time.RFC3339)
				if quota, exists := policy["quotaGiB"]; exists {
					row["quotaGiB"] = quota
				}
				next, e = c.NextReset(policy, next)
				if e != nil {
					return e
				}
			}
			policy["nextAt"] = next.UTC().Format(time.RFC3339)
			row["resetPolicy"] = policy
			updates = append(updates, row)
			continue
		}
		days := int(p.Number(row, "cycleDays"))
		if days == 0 {
			continue
		}
		start, err := time.Parse(time.RFC3339, p.Text(row, "cycleStart"))
		if err == nil && !start.AddDate(0, 0, days).After(time.Now()) {
			elapsed := int(time.Since(start).Hours()/24) / days
			row["cycleStart"] = start.AddDate(0, 0, elapsed*days).UTC().Format(time.RFC3339)
			updates = append(updates, row)
		}
	}
	for offset := 0; offset < len(updates); offset += 200 {
		if err := a.C.PutMany("accounts", updates[offset:min(offset+200, len(updates))]); err != nil {
			return err
		}
	}
	return nil
}

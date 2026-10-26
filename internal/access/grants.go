package access

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"strings"
	"time"
)

var entitlementFields = []string{"expiresAt", "quotaGiB", "unlimited", "groups", "cycleStart", "cycleDays", "resetPolicy", "status", "accessLevel", "levelExpiresAt", "speedLimitMbps", "deviceLimit", "planFamilyId"}

func addMonths(value time.Time, months int) time.Time {
	start := time.Date(value.Year(), value.Month()+time.Month(months), 1, value.Hour(), value.Minute(), value.Second(), 0, value.Location())
	last := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, value.Location()).Day()
	return time.Date(start.Year(), start.Month(), min(value.Day(), last), value.Hour(), value.Minute(), value.Second(), 0, value.Location())
}

func (a *app) grant(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "commerce", "redeem") {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if c.Err(w, c.Validate(b, "userId", "grantId")) {
		return
	}
	rows, e := a.C.List("accounts")
	if c.Err(w, e) {
		return
	}
	var row p.Record
	for _, v := range rows {
		if v["userId"] == b["userId"] {
			row = v
		}
	}
	if row == nil {
		if c.Allowed(p.Text(b, "kind"), "traffic", "reset", "time") {
			p.Fail(w, 400, "请先开通基础套餐")
			return
		}
		row = p.Record{"id": p.ID(), "userId": b["userId"], "numericId": nextNumericID(rows), "credential": uuid(), "cycleStart": cycleNow(), "grants": map[string]any{}}
	}
	grants, _ := row["grants"].(map[string]any)
	if grants == nil {
		grants = map[string]any{}
	}
	if _, ok := grants[p.Text(b, "grantId")]; ok {
		p.JSON(w, row)
		return
	}
	sequence := 0.0
	for _, raw := range grants {
		grant, _ := raw.(map[string]any)
		sequence = max(sequence, p.Number(grant, "sequence"))
	}
	previous := p.Record{"createdAt": c.Now(), "kind": b["kind"], "sequence": sequence + 1}
	for _, key := range entitlementFields {
		previous[key] = row[key]
	}
	switch p.Text(b, "kind") {
	case "traffic":
		row["quotaGiB"] = p.Number(row, "quotaGiB") + p.Number(b, "quotaGiB")
	case "reset":
		row["cycleStart"] = cycleNow()
	default:
		base := time.Now()
		if expires, e := time.Parse(time.RFC3339, p.Text(row, "expiresAt")); e == nil && expires.After(base) {
			base = expires
		}
		zoneName := p.Text(b, "timezone")
		if zoneName == "" {
			zoneName = "UTC"
		}
		zone, err := time.LoadLocation(zoneName)
		if c.Err(w, err) {
			return
		}
		base = base.In(zone)
		unit, count := p.Text(b, "durationUnit"), int(p.Number(b, "durationValue"))
		if unit == "permanent" {
			row["expiresAt"] = ""
		} else if unit == "month" && count > 0 {
			row["expiresAt"] = addMonths(base, count).UTC().Format(time.RFC3339)
		} else if unit == "day" && count > 0 {
			row["expiresAt"] = base.AddDate(0, 0, count).UTC().Format(time.RFC3339)
		} else {
			p.Fail(w, 400, "套餐缺少有效期限")
			return
		}
		if p.Text(b, "kind") != "time" {
			if p.Text(row, "planFamilyId") != p.Text(b, "planFamilyId") || p.Text(b, "planFamilyId") == "" {
				row["cycleStart"] = cycleNow()
			}
			for _, key := range []string{"quotaGiB", "unlimited", "groups", "accessLevel", "speedLimitMbps", "deviceLimit", "planFamilyId"} {
				if b[key] != nil {
					row[key] = b[key]
				}
			}
			row["cycleDays"] = 30.0
			if policy, ok := b["resetPolicy"].(map[string]any); ok {
				if p.Text(policy, "timezone") == "" {
					policy["timezone"] = zoneName
				}
				policyZone, err := time.LoadLocation(p.Text(policy, "timezone"))
				if c.Err(w, err) {
					return
				}
				row["cycleDays"] = 0
				delete(policy, "nextAt")
				if strings.HasSuffix(p.Text(policy, "type"), "Anniversary") {
					if expires, e := time.Parse(time.RFC3339, p.Text(row, "expiresAt")); e == nil {
						expires = expires.In(policyZone)
						policy["day"] = expires.Day()
						policy["month"] = int(expires.Month())
						if p.Bool(policy, "anchorTime") {
							policy["hour"] = expires.Hour()
							policy["minute"] = expires.Minute()
							policy["second"] = expires.Second()
						}
					}
				}
				row["resetPolicy"] = policy
			} else {
				delete(row, "resetPolicy")
			}
		}
		if b["accessLevel"] != nil {
			row["accessLevel"] = b["accessLevel"]
			row["levelExpiresAt"] = row["expiresAt"]
		}
		row["status"] = "active"
	}
	grants[p.Text(b, "grantId")] = previous
	row["grants"] = grants
	if !c.Err(w, a.C.Put("accounts", row)) {
		p.JSON(w, row)
	}
}
func (a *app) revoke(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "commerce") {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	rows, e := a.C.List("accounts")
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		if row["userId"] != b["userId"] {
			continue
		}
		grants, _ := row["grants"].(map[string]any)
		grant, _ := grants[p.Text(b, "grantId")].(map[string]any)
		if grant == nil || grant["revoked"] == true {
			p.JSON(w, true)
			return
		}
		latest := true
		for _, g := range grants {
			v, _ := g.(map[string]any)
			if v != nil && v["revoked"] != true && (p.Number(v, "sequence") > p.Number(grant, "sequence") || p.Number(v, "sequence") == p.Number(grant, "sequence") && p.Text(v, "createdAt") > p.Text(grant, "createdAt")) {
				latest = false
			}
		}
		if !latest {
			p.Fail(w, 400, "请先退回该账户较新的订单")
			return
		}
		for _, k := range entitlementFields {
			row[k] = grant[k]
		}
		if row["status"] == nil {
			row["status"] = "suspended"
		}
		grant["revoked"] = true
		grants[p.Text(b, "grantId")] = grant
		row["grants"] = grants
		if !c.Err(w, a.C.Put("accounts", row)) {
			p.JSON(w, true)
		}
		return
	}
	p.JSON(w, true)
}

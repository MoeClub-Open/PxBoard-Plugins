package access

import (
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) reminderConfig() (p.Record, error) {
	rows, err := a.C.List("reminder-settings")
	if err != nil || len(rows) == 0 {
		return p.Record{"id": "settings", "expireEnabled": false, "trafficEnabled": false, "expireDays": 3}, err
	}
	return rows[0], nil
}

func (a *app) reminderSettings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, err := a.reminderConfig()
	if c.Err(w, err) {
		return
	}
	if r.Method == "PATCH" {
		var body struct {
			ExpireEnabled, TrafficEnabled bool
			ExpireDays                    int
		}
		if c.Err(w, p.Decode(r, &body)) {
			return
		}
		if body.ExpireDays < 1 {
			p.Fail(w, 400, "到期提醒天数须为正整数")
			return
		}
		row["expireEnabled"], row["trafficEnabled"], row["expireDays"] = body.ExpireEnabled, body.TrafficEnabled, body.ExpireDays
		if c.Err(w, a.C.Put("reminder-settings", row)) {
			return
		}
	}
	p.JSON(w, row)
}

func (a *app) preferences(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	user := p.Subject(r).ID
	rows, err := a.recordsFor("preferences", "id", user)
	if c.Err(w, err) {
		return
	}
	row := p.Record{"id": user, "expireReminder": true, "trafficReminder": true}
	if len(rows) > 0 {
		row = rows[0]
	}
	if r.Method == "PATCH" {
		var b struct{ ExpireReminder, TrafficReminder *bool }
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if b.ExpireReminder != nil {
			row["expireReminder"] = *b.ExpireReminder
		}
		if b.TrafficReminder != nil {
			row["trafficReminder"] = *b.TrafficReminder
		}
		if c.Err(w, a.C.Put("preferences", row)) {
			return
		}
	}
	config, err := a.reminderConfig()
	if !c.Err(w, err) {
		p.JSON(w, p.Record{"expireReminder": row["expireReminder"], "trafficReminder": row["trafficReminder"], "expireEnabled": config["expireEnabled"], "trafficEnabled": config["trafficEnabled"]})
	}
}

// The expiry and cycle are the identities of a reminder, so retries and ticks
// cannot send it twice. Renewal and a traffic reset naturally start new identities.
func reminderKind(row, config, prefs p.Record, now time.Time) string {
	if p.Text(row, "status") != "active" {
		return ""
	}
	expiry, err := time.Parse(time.RFC3339, p.Text(row, "expiresAt"))
	if p.Text(row, "expiresAt") != "" && (err != nil || !expiry.After(now)) {
		return ""
	}
	if p.Bool(config, "expireEnabled") && prefs["expireReminder"] != false && !expiry.IsZero() && !expiry.After(now.Add(time.Duration(p.Number(config, "expireDays"))*24*time.Hour)) {
		return "expiry"
	}
	return ""
}

func trafficDepleted(row, config, prefs p.Record, now time.Time) bool {
	if !p.Bool(config, "trafficEnabled") || prefs["trafficReminder"] == false || p.Text(row, "status") != "active" || p.Bool(row, "unlimited") || p.Number(row, "quotaGiB") <= 0 {
		return false
	}
	if expiry := p.Text(row, "expiresAt"); expiry != "" {
		t, err := time.Parse(time.RFC3339, expiry)
		if err != nil || !t.After(now) {
			return false
		}
	}
	return p.Number(row, "usedGiB") >= p.Number(row, "quotaGiB")
}

func reminderKey(kind string, row p.Record) string {
	boundary := p.Text(row, "expiresAt")
	if kind == "traffic" {
		boundary = p.Text(row, "cycleStart")
	}
	return kind + ":" + p.Text(row, "userId") + ":" + boundary
}

func (a *app) sendReminders() error {
	config, err := a.reminderConfig()
	if err != nil || !p.Bool(config, "expireEnabled") && !p.Bool(config, "trafficEnabled") {
		return err
	}
	rows, err := a.enrichedFor("")
	if err != nil {
		return err
	}
	prefs, err := a.C.List("preferences")
	if err != nil {
		return err
	}
	byUser := map[string]p.Record{}
	for _, row := range prefs {
		byUser[p.Text(row, "id")] = row
	}
	users, err := a.UsersFor(rows, "userId")
	if err != nil {
		return err
	}
	active := map[string]bool{}
	for _, u := range users {
		active[p.Text(u, "id")] = p.Text(u, "status") == "active"
	}
	known, err := a.C.List("reminders")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, row := range known {
		seen[p.Text(row, "id")] = true
	}
	now := time.Now()
	for _, row := range rows {
		user := p.Text(row, "userId")
		if !active[user] {
			continue
		}
		for _, kind := range []string{"expiry", "traffic"} {
			if kind == "expiry" && reminderKind(row, config, byUser[user], now) != "expiry" || kind == "traffic" && !trafficDepleted(row, config, byUser[user], now) {
				continue
			}
			id := reminderKey(kind, row)
			if seen[id] {
				continue
			}
			data := p.Record{"reminderId": id, "userId": user, "kind": kind, "expiresAt": row["expiresAt"], "cycleStart": row["cycleStart"], "quotaGiB": row["quotaGiB"], "usedGiB": row["usedGiB"]}
			if err = a.C.Commit("reminder:"+id, []p.Write{{Collection: "reminders", Record: p.Record{"id": id, "userId": user, "createdAt": c.Now()}}}, p.Emission{Type: "access.reminder", Data: data}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *app) reminderValid(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "notifications") {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	config, err := a.reminderConfig()
	if c.Err(w, err) {
		return
	}
	rows, err := a.enrichedFor(p.Text(b, "userId"))
	if c.Err(w, err) {
		return
	}
	prefs, err := a.recordsFor("preferences", "id", p.Text(b, "userId"))
	if c.Err(w, err) {
		return
	}
	preference := p.Record{}
	if len(prefs) > 0 {
		preference = prefs[0]
	}
	valid := false
	if len(rows) > 0 {
		row := rows[0]
		users, err := a.C.UsersByID(p.Text(b, "userId"))
		if c.Err(w, err) {
			return
		}
		active := len(users) > 0 && p.Text(users[0], "status") == "active"
		kind := p.Text(b, "kind")
		valid = active && reminderKey(kind, row) == p.Text(b, "reminderId") && (kind == "expiry" && reminderKind(row, config, preference, time.Now()) == "expiry" || kind == "traffic" && trafficDepleted(row, config, preference, time.Now()))
	}
	p.JSON(w, p.Record{"valid": valid})
}

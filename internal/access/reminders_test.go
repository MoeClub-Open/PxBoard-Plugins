package access

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"testing"
	"time"
)

func TestReminderEligibility(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	config := p.Record{"expireEnabled": true, "trafficEnabled": true, "expireDays": 3}
	row := p.Record{"userId": "u", "status": "active", "expiresAt": "2026-10-30T00:00:00Z", "cycleStart": "cycle1", "quotaGiB": 100, "usedGiB": 100}
	if reminderKind(row, config, nil, now) != "expiry" || !trafficDepleted(row, config, nil, now) {
		t.Fatal("eligible reminder omitted")
	}
	first := reminderKey("traffic", row)
	row["cycleStart"] = "cycle2"
	if first == reminderKey("traffic", row) {
		t.Fatal("cycles share a notification identity")
	}
	if trafficDepleted(row, config, p.Record{"trafficReminder": false}, now) || reminderKind(row, config, p.Record{"expireReminder": false}, now) != "" {
		t.Fatal("user opt-out ignored")
	}
	row["quotaGiB"] = 0
	if trafficDepleted(row, config, nil, now) {
		t.Fatal("unopened account counted as exhausted")
	}
	row["quotaGiB"] = 100
	row["unlimited"] = true
	if trafficDepleted(row, config, nil, now) {
		t.Fatal("unlimited account counted as exhausted")
	}
	row["expiresAt"] = "2026-01-01T00:00:00Z"
	if reminderKind(row, config, nil, now) != "" {
		t.Fatal("historical expired user notified")
	}
}

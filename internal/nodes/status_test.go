package nodes

import (
	"testing"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestServerMonitoringAggregatesByServerAndHidesStaleOfflineTelemetry(t *testing.T) {
	now := time.Now().UTC()
	recent := now.Add(-30 * time.Second).Format(time.RFC3339)
	stale := now.Add(-48 * time.Hour).Format(time.RFC3339)

	machines := []p.Record{
		{
			"id":              "m2",
			"number":          2.0,
			"name":            "Offline HK",
			"enabled":         true,
			"lastSeen":        stale,
			"statusUpdatedAt": stale,
			"status":          map[string]any{"cpuPercent": 88.0, "memory": map[string]any{"used": 80.0, "total": 100.0}},
		},
		{
			"id":              "m1",
			"number":          1.0,
			"name":            "Online JP",
			"region":          "日本",
			"enabled":         true,
			"lastSeen":        recent,
			"statusUpdatedAt": recent,
			"status": map[string]any{
				"cpuPercent": 12.34,
				"memory":     map[string]any{"used": 512.0, "total": 1024.0},
				"disk":       map[string]any{"used": 25.0, "total": 100.0},
			},
		},
		{"id": "m3", "number": 3.0, "name": "Disabled US", "enabled": false, "lastSeen": recent},
	}
	nodes := []p.Record{
		{
			"id":              "n1",
			"enabled":         true,
			"visible":         true,
			"engine":          "noders",
			"machineId":       1.0,
			"groups":          "vip",
			"onlineUpdatedAt": recent,
			"onlineAccounts":  []any{map[string]any{"accountId": 101.0, "ips": []any{"1.1.1.1"}}},
		},
		{
			"id":              "n2",
			"enabled":         true,
			"visible":         false,
			"engine":          "noders",
			"machineId":       1.0,
			"groups":          "vip",
			"onlineUpdatedAt": recent,
			"onlineAccounts": []any{
				map[string]any{"accountId": 101.0, "ips": []any{"1.1.1.1"}},
				map[string]any{"accountId": 102.0, "ips": []any{"2.2.2.2"}},
			},
		},
		{"id": "n1-v6", "enabled": false, "visible": true, "parentId": "n1", "groups": "member"},
	}

	if count := onlineServerCount(machines, now); count != 1 {
		t.Fatalf("onlineServerCount = %d, want 1", count)
	}

	adminRows, totalOnlineUsers := buildServerStatuses(machines, nodes, nil, false, nil, now)
	if len(adminRows) != 3 || adminRows[0]["id"] != "m1" || adminRows[1]["id"] != "m2" || adminRows[2]["id"] != "m3" {
		t.Fatalf("unexpected server order or count: %#v", adminRows)
	}
	if totalOnlineUsers != 2 || adminRows[0]["onlineAccounts"] != 2 || adminRows[0]["nodeCount"] != 2 {
		t.Fatalf("expected deduplicated onlineAccounts=2 and nodeCount=2, got %#v (total %d)", adminRows[0], totalOnlineUsers)
	}
	if adminRows[0]["cpuPercent"] != 12.3 || adminRows[0]["memoryPercent"] != 50.0 {
		t.Fatalf("unexpected rounded load metrics: %#v", adminRows[0])
	}
	if adminRows[1]["status"] != "offline" || adminRows[1]["cpuPercent"] != nil || adminRows[1]["memoryPercent"] != nil {
		t.Fatalf("offline server leaked stale load metrics: %#v", adminRows[1])
	}
	if adminRows[2]["status"] != "disabled" || adminRows[2]["nodeCount"] != 0 {
		t.Fatalf("disabled server state mismatch: %#v", adminRows[2])
	}

	// Member with vip+member access can see m1 via shared address n1-v6, filtered to configured fields, and never sees disabled m3 or unlinked m2.
	memberAccounts := []p.Record{{"groups": "vip,member"}}
	memberRows, _ := buildServerStatuses(machines, nodes, memberAccounts, true, []string{"status", "cpuPercent"}, now)
	if len(memberRows) != 1 || memberRows[0]["id"] != "m1" || memberRows[0]["cpuPercent"] != 12.3 || memberRows[0]["memoryPercent"] != nil {
		t.Fatalf("unexpected member server status rows: %#v", memberRows)
	}
}

func TestSelectedStatusFieldsDropsLegacyNodeFields(t *testing.T) {
	fields := selectedStatusFields(p.Record{"userStatusFields": []any{"region", "protocol", "status", "trafficRate", "lastSeen"}})
	if len(fields) != 3 || fields[0] != "region" || fields[1] != "status" || fields[2] != "lastSeen" {
		t.Fatalf("unexpected sanitized fields: %#v", fields)
	}
	fallback := selectedStatusFields(p.Record{"userStatusFields": []string{"protocol", "trafficRate"}})
	if len(fallback) != len(defaultStatusFields) {
		t.Fatalf("expected defaultStatusFields fallback, got %#v", fallback)
	}
}

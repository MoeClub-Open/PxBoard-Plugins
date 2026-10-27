package nodes

import (
	"testing"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestOnlineCountCountsPrimaryNodes(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	nodes := []p.Record{
		{"id": "primary", "enabled": true, "engine": "noders", "machineId": 1.0, "lastSeen": now},
		{"id": "alias", "enabled": false, "parentId": "primary"},
		{"id": "independent", "enabled": true, "parentId": "primary", "lastSeen": now},
		{"id": "error", "enabled": true, "lastSeen": now, "error": "failed"},
		{"id": "disabled-machine", "enabled": true, "engine": "noders", "machineId": 2.0, "lastSeen": now},
		{"id": "orphan", "enabled": false, "parentId": "missing"},
	}
	machines := []p.Record{{"number": 1.0, "enabled": true}, {"number": 2.0, "enabled": false}}
	if count := onlineCount(nodes, machines); count != 2 {
		t.Fatalf("onlineCount = %d, want 2", count)
	}
}

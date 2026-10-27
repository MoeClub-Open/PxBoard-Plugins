package nodes

import (
	"encoding/json"
	"testing"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestSharedAddressUsesParentServiceAndPermissions(t *testing.T) {
	parent := p.Record{"id": "parent", "enabled": true, "protocol": "anytls", "passwordMode": "uuid", "serverName": "parent.example", "groups": "allowed", "minimumAccessLevel": 2.0}
	child := p.Record{"id": "child", "parentId": "parent", "enabled": false, "host": "v6.example", "groups": "allowed,other", "protocol": "old"}
	result := subscriptionNode(child, parent)
	if result["host"] != "v6.example" || result["protocol"] != "anytls" || result["passwordMode"] != "uuid" || child["protocol"] != "old" {
		t.Fatal("shared address did not inherit service settings without changing stored data")
	}
	raw, _ := json.Marshal(result)
	var decoded p.Record
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	parentAccess := p.Record(decoded["parentAccess"].(map[string]any))
	if !c.CanUse(p.Record{"groups": "allowed", "accessLevel": 2.0}, parentAccess) || c.CanUse(p.Record{"groups": "other", "accessLevel": 2.0}, parentAccess) || c.CanUse(p.Record{"groups": "allowed", "accessLevel": 1.0}, parentAccess) {
		t.Fatal("parent access restrictions were not preserved")
	}
}

package nodes

import (
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestBuildMachineNodeConfigAndUsers(t *testing.T) {
	node := p.Record{
		"id":           "n1",
		"wireId":       7.0,
		"protocol":     "anytls",
		"host":         "jp.example.com",
		"port":         443.0,
		"serverPort":   8443.0,
		"tls":          true,
		"serverName":   "jp.example.com",
		"groups":       "vip",
		"certPath":     "/etc/ssl/cert.pem",
		"keyPath":      "/etc/ssl/key.pem",
		"passwordMode": "uuid",
	}
	cfg := buildMachineNodeConfig(node)
	listen := cfg["listen"].(p.Record)
	if listen["port"] != 8443.0 {
		t.Fatalf("expected serverPort 8443, got %v", listen["port"])
	}
	clients := []p.Record{
		{"numericId": 20.0, "userId": "u2", "email": "u2@example.com", "credential": "uuid-2", "groups": "vip", "speedLimitMbps": 100.0},
		{"numericId": 10.0, "userId": "u1", "email": "u1@example.com", "credential": "uuid-1", "groups": "vip"},
		{"numericId": 30.0, "userId": "u3", "email": "u3@example.com", "credential": "uuid-3", "groups": "basic"},
	}
	users := buildNodeUsers(clients, node)
	if len(users) != 2 || users[0]["accountId"] != 10.0 || users[1]["accountId"] != 20.0 {
		t.Fatalf("expected sorted matching users [10, 20], got %#v", users)
	}
}

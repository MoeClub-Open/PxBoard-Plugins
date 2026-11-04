package nodes

import (
	"encoding/json"
	"strings"
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestFluxNodeNormalizeMergeAndSanitize(t *testing.T) {
	// Known clamped 32-byte X25519 private key (RFC 7748 test scalar 0x77076d0a...)
	privB64 := "dwdtCnMYpX08FsFyUbJmRd9ML4frwJkqsXf7pR25LCo"
	expectedPubB64 := "hSDwCYkwp1R0i33ctD73Wg2_Og0mOBr066SpjqqbTmo"

	node := p.Record{
		"name":       "HK-Flux",
		"host":       "entry.example.com",
		"port":       443.0,
		"serverPort": 8443.0,
		"engine":     "noders",
		"protocol":   "flux",
		"protocolOptions": map[string]any{
			"security":            "reality",
			"ipStrategy":          "dual",
			"standbyChannels":     2.0,
			"rotationIntervalSec": 180.0,
			"rotationBytesMb":     96.0,
			"reality": map[string]any{
				"serverName": "www.microsoft.com",
				"dest":       "www.microsoft.com:443",
				"privateKey": privB64,
				"shortId":    "01234567",
			},
			"channels": []any{
				map[string]any{
					"id":           "direct-reality",
					"enabled":      true,
					"connectPorts": "443",
					"listenIp":     "0.0.0.0",
					"listenPorts":  "8443",
					"transport":    "udp",
					"security":     "",
					"role":         "primary",
					"weight":       100.0,
				},
				map[string]any{
					"id":           "cdn-ws",
					"enabled":      true,
					"connectHost":  "cdn.example.com",
					"connectPorts": "443",
					"listenIp":     "127.0.0.1",
					"listenPorts":  "18080",
					"transport":    "http",
					"httpMode":     "ws",
					"httpHost":     "cdn.example.com",
					"httpPath":     "flux-ws",
					"httpHeaders":  map[string]any{"X-Edge": "hk"},
					"security":     "tls",
					"certMode":     "origin_plain",
					"sni":          "cdn.example.com",
					"role":         "backup",
					"weight":       80.0,
				},
				map[string]any{
					"id":           "range-obfs",
					"enabled":      true,
					"connectPorts": "20000-20009",
					"transport":    "udp",
					"security":     "obfs",
				},
				map[string]any{
					"id":      "disabled-ch",
					"enabled": false,
				},
			},
		},
	}

	if err := normalizeNodeConnection(node, true, false, false); err != nil {
		t.Fatalf("normalize Flux node failed: %v", err)
	}

	opts := node["protocolOptions"].(map[string]any)
	nodeReality := opts["reality"].(map[string]any)
	if nodeReality["publicKey"] != expectedPubB64 {
		t.Fatalf("unexpected derived X25519 public key: got %v, want %s", nodeReality["publicKey"], expectedPubB64)
	}

	channels := opts["channels"].([]any)
	if len(channels) != 4 {
		t.Fatalf("expected 4 stored channels, got %d", len(channels))
	}
	obfsCh := channels[2].(map[string]any)
	if len(p.Text(obfsCh, "obfsKey")) != 32 {
		t.Fatalf("expected 32-char auto-generated obfsKey, got %q", p.Text(obfsCh, "obfsKey"))
	}
	if _, hasEmptySec := channels[0].(map[string]any)["security"]; hasEmptySec {
		t.Fatal("inherited channel security must be omitted, not empty string")
	}

	// Verify machineConfig output keeps server-side fields and omits empty enums
	sec, machineOpts := fluxMachineSecurityAndOptions(node)
	if sec["mode"] != "reality" {
		t.Fatalf("expected machine security mode reality, got %v", sec["mode"])
	}
	machineRaw, _ := json.Marshal(machineOpts)
	if !strings.Contains(string(machineRaw), privB64) || !strings.Contains(string(machineRaw), "origin_plain") {
		t.Fatalf("machineConfig lost server fields: %s", string(machineRaw))
	}

	// Verify subscription sanitization strips all server secrets & disabled channels without mutating node
	sanitized := sanitizeFluxSubscriptionNode(node)
	subRawBytes, _ := json.Marshal(sanitized)
	subRaw := string(subRawBytes)
	for _, forbidden := range []string{privB64, "privateKey", "private_key", "dest", "listenIp", "listenPorts", "certMode", "origin_plain", "serverPort", "disabled-ch"} {
		if strings.Contains(subRaw, forbidden) {
			t.Fatalf("sanitized subscription node leaked %q: %s", forbidden, subRaw)
		}
	}
	if !strings.Contains(subRaw, expectedPubB64) {
		t.Fatalf("sanitized subscription node missing derived publicKey: %s", subRaw)
	}
	// Ensure original node was not mutated
	if node["protocolOptions"].(map[string]any)["reality"].(map[string]any)["privateKey"] != privB64 {
		t.Fatal("sanitizing subscription node mutated stored node record")
	}

	// Verify mergeNodeSettings replaces channels slice as a whole when deleting channels
	patch := p.Record{
		"protocolOptions": map[string]any{
			"channels": []any{channels[0]},
		},
	}
	mergeNodeSettings(patch, node)
	mergedChannels := patch["protocolOptions"].(map[string]any)["channels"].([]any)
	if len(mergedChannels) != 1 {
		t.Fatalf("expected channels slice replacement to leave 1 channel, got %d", len(mergedChannels))
	}

	// Verify > 64 expanded ports budget is rejected
	overBudget := p.Record{
		"name":     "OverBudget",
		"host":     "entry.example.com",
		"port":     443.0,
		"engine":   "noders",
		"protocol": "flux",
		"protocolOptions": map[string]any{
			"security": "obfs",
			"channels": []any{
				map[string]any{"id": "too-many", "connectPorts": "10000-10064", "transport": "udp"},
			},
		},
	}
	if err := normalizeNodeConnection(overBudget, true, false, false); err == nil {
		t.Fatal("expected > 64 ports to fail validation")
	}
}

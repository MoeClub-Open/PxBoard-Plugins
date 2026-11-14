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
		"protocol":   "flux",
		"protocolOptions": map[string]any{
			"security":            "reality",
			"ipStrategy":          "dual",
			"standbyChannels":     2.0,
			"rotationIntervalSec": 180.0,
			"rotationBytesMb":     96.0,
			"reality": map[string]any{
				"serverName": "huggingface.co",
				"dest":       "huggingface.co:443",
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

func TestFluxDefaultsToTCPObfsAndDerivesEndpointFromChannels(t *testing.T) {
	node := p.Record{
		"name":      "SG-Flux",
		"machineId": 1.0,
		"protocol":  "flux",
		"protocolOptions": map[string]any{
			"channels": []any{
				map[string]any{
					"id":           "tcp-main",
					"enabled":      true,
					"connectHost":  "sg.example.com",
					"connectPorts": "25566",
					"listenPorts":  "25567",
					"transport":    "tcp",
					"role":         "primary",
				},
				map[string]any{
					"id":           "udp-follow",
					"enabled":      true,
					"connectPorts": "20000-20005",
					"transport":    "udp",
					"role":         "primary",
				},
			},
		},
	}

	if err := normalizeNodeConnection(node, true, false, false, false, false); err != nil {
		t.Fatalf("normalize channel-first Flux node failed: %v", err)
	}
	if node["host"] != "sg.example.com" || node["port"] != 25566.0 || node["serverPort"] != nil {
		t.Fatalf("unexpected derived Flux endpoint: host=%v port=%v serverPort=%v", node["host"], node["port"], node["serverPort"])
	}
	if node["tls"] != false || node["tlsMode"] != "none" {
		t.Fatalf("expected Flux default security obfs without tls, got tls=%v tlsMode=%v", node["tls"], node["tlsMode"])
	}
	opts := node["protocolOptions"].(map[string]any)
	if opts["security"] != "obfs" || len(p.Text(opts, "obfsKey")) != 32 {
		t.Fatalf("expected protocolOptions.security=obfs with 32-char obfsKey, got security=%v obfsKey=%q", opts["security"], p.Text(opts, "obfsKey"))
	}
	channels := opts["channels"].([]any)
	if _, hasConnectHost := channels[1].(map[string]any)["connectHost"]; hasConnectHost {
		t.Fatal("secondary channel with blank connectHost should inherit primary host rather than duplicating it")
	}

	// Verify REALITY defaults serverName to huggingface.co and removing REALITY clears realitySettings
	realityNode := p.Record{
		"name":      "HF-Flux",
		"machineId": 1.0,
		"protocol":  "flux",
		"protocolOptions": map[string]any{
			"security": "reality",
			"channels": []any{
				map[string]any{
					"id":           "tcp-main",
					"enabled":      true,
					"connectHost":  "hf.example.com",
					"connectPorts": "18444",
					"transport":    "tcp",
				},
			},
		},
	}
	if err := normalizeNodeConnection(realityNode, true, false, false, false, false); err != nil {
		t.Fatalf("normalize REALITY Flux node failed: %v", err)
	}
	rOpts := realityNode["protocolOptions"].(map[string]any)["reality"].(map[string]any)
	if rOpts["serverName"] != "huggingface.co" {
		t.Fatalf("expected default REALITY serverName huggingface.co, got %v", rOpts["serverName"])
	}
	switchPatch := p.Record{
		"name":      "HF-Flux",
		"machineId": 1.0,
		"protocol":  "flux",
		"protocolOptions": map[string]any{
			"security": "obfs",
		},
	}
	mergeNodeSettings(switchPatch, realityNode)
	if err := normalizeNodeConnection(switchPatch, false, false, false, false, false); err != nil {
		t.Fatalf("switch REALITY Flux node to obfs failed: %v", err)
	}
	if _, exists := switchPatch["protocolOptions"].(map[string]any)["reality"]; exists {
		t.Fatal("expected protocolOptions.reality removed after switching away from REALITY")
	}
	if switchPatch["realitySettings"] != nil {
		t.Fatalf("expected realitySettings nil after switching away from REALITY, got %#v", switchPatch["realitySettings"])
	}

	legacy := materializeFluxNodeForView(p.Record{
		"name":       "Legacy-Flux",
		"protocol":   "flux",
		"host":       "legacy.example.com",
		"port":       25566.0,
		"serverPort": 10443.0,
	})
	legacyCh := legacy["protocolOptions"].(map[string]any)["channels"].([]any)
	if len(legacyCh) != 1 || legacyCh[0].(map[string]any)["transport"] != "tcp" || legacyCh[0].(map[string]any)["connectHost"] != "legacy.example.com" || legacyCh[0].(map[string]any)["listenPorts"] != "10443" {
		t.Fatalf("unexpected materialized legacy Flux channels: %#v", legacyCh)
	}

	res := resource()
	if len(res.Actions) != 0 {
		t.Fatalf("expected legacy rotate agentToken action removed, got %#v", res.Actions)
	}
	for _, f := range res.Fields {
		if f.Key == "engine" || f.Key == "certPath" || f.Key == "keyPath" {
			t.Fatalf("unexpected legacy field %q in node form", f.Key)
		}
		if f.Key == "protocolOptions.security" {
			if f.Default != "obfs" || len(f.Options) != 3 || f.Options[0].Value != "obfs" || f.Options[2].Value != "reality" || f.Options[2].Label != "REALITY" {
				t.Fatalf("unexpected Flux security field options/default: %#v", f)
			}
		}
		if f.Key == "protocolOptions.reality.serverName" && f.Default != "huggingface.co" {
			t.Fatalf("expected REALITY serverName default huggingface.co, got %#v", f.Default)
		}
	}
}

package subscriptions

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestAerionSubscriptionAdapterSelection(t *testing.T) {
	for _, item := range []struct{ query, agent, format string }{
		{"?format=aerion", "Mihomo/1.19.22", "aerion"},
		{"?format=xbclient", "", "aerion"},
		{"?flag=aerion", "", "aerion"},
		{"?flag=xbclient", "", "aerion"},
		{"", "Aerion/0.1", "aerion"},
		{"", "XBClient/0.1", "aerion"},
		{"?format=clash&types=flux", "Aerion/0.1", "clash"},
		{"?flag=meta&types=flux", "Aerion/0.1", "clash"},
		{"?format=sing-box", "Aerion/0.1", "sing-box"},
		{"?types=all", "Mihomo/1.19.22", "clash"},
	} {
		t.Run(item.query+item.agent, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/s/token"+item.query, nil)
			r.Header.Set("User-Agent", item.agent)
			if got := subscriptionFormat(r); got != item.format {
				t.Fatalf("adapter = %q, want %q", got, item.format)
			}
			if item.format == "clash" && supportedFormat(p.Record{"protocol": "flux"}, item.format, r) {
				t.Fatal("Mihomo adapter must reject Flux regardless of capabilities")
			}
		})
	}
}

func TestAerionReusesStandardProtocolSerialization(t *testing.T) {
	node := p.Record{"name": "VLESS", "protocol": "vless", "host": "edge.example.com", "port": 443,
		"transport": "ws", "transportSettings": map[string]any{"path": "/ws"}}
	if !reflect.DeepEqual(aerion(node, "password", "user-id"), clash(node, "password", "user-id")) {
		t.Fatal("standard protocol fields must remain unchanged")
	}
}

func TestSanitizeAerionYAMLStripsMrsRuleProviders(t *testing.T) {
	proxy := aerion(p.Record{
		"name":     "JP-AnyTLS",
		"protocol": "anytls",
		"host":     "jp.example.com",
		"port":     443.0,
		"tls":      true,
		"tlsSettings": map[string]any{
			"allow_insecure": true,
		},
	}, "secret", "uuid-1")
	rendered, err := sc.Render("clash", sc.Default("clash"), []map[string]any{map[string]any(proxy)}, "PxBoard", []string{"secapi.telecom.moe"})
	if err != nil {
		t.Fatalf("render default clash template: %v", err)
	}
	sanitized, routing, err := sanitizeAerionYAML(rendered)
	if err != nil {
		t.Fatalf("sanitizeAerionYAML: %v", err)
	}
	yamlText := string(sanitized)
	if strings.Contains(yamlText, "rule-providers:") || strings.Contains(yamlText, ".mrs") || strings.Contains(yamlText, "RULE-SET,") || strings.Contains(yamlText, "rule-set:cn-domain") {
		t.Fatalf("expected binary mrs rule-providers and references to be stripped, got:\n%s", yamlText)
	}
	if !strings.Contains(yamlText, "DOMAIN-SUFFIX,cn,DIRECT") {
		t.Fatalf("expected DOMAIN-SUFFIX,cn,DIRECT replacement, got:\n%s", yamlText)
	}
	if !routing.HasRules || routing.RuleProviderCount != 0 || routing.ProxyGroupCount != 3 {
		t.Fatalf("unexpected routing summary: %+v", routing)
	}
	clientNode := aerionClientNode(p.Record{
		"name":     "JP-AnyTLS",
		"protocol": "anytls",
		"host":     "jp.example.com",
		"port":     443.0,
		"tls":      true,
		"tlsSettings": map[string]any{
			"allow_insecure": true,
		},
	}, "secret", "uuid-1")
	if clientNode["host"] != "jp.example.com" || clientNode["insecure"] != true {
		t.Fatalf("expected host and insecure aliases on client node: %#v", clientNode)
	}
}

func TestSummarizeClientAccountStates(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	noneSummary, reason := summarizeClientAccount(nil, now)
	if reason != "no_plan" || noneSummary["exists"] != false {
		t.Fatalf("expected no_plan for nil account, got %q %#v", reason, noneSummary)
	}
	unlimitedSummary, reason := summarizeClientAccount(p.Record{
		"status":    "active",
		"effective": true,
		"unlimited": true,
		"usedBytes": 5368709120.0,
		"quotaGiB":  0.0,
	}, now)
	if reason != "" || unlimitedSummary["unlimited"] != true || unlimitedSummary["usedBytes"] != int64(5368709120) {
		t.Fatalf("unexpected unlimited summary: %q %#v", reason, unlimitedSummary)
	}
}

func TestAerionClientNodeEmitsObjectTLSAndNormalizedProtocolFields(t *testing.T) {
	anytlsNoSNI := aerionClientNode(p.Record{
		"name":     "JP Osaka Oracle",
		"protocol": "anytls",
		"host":     "osa.example.com",
		"port":     443.0,
		"tls":      true,
		"tlsMode":  "tls",
	}, "secret", "uuid-1")
	tlsMap, ok := anytlsNoSNI["tls"].(p.Record)
	if !ok || tlsMap["enabled"] != true {
		t.Fatalf("expected AnyTLS tls object with enabled=true, got %#v", anytlsNoSNI["tls"])
	}
	if _, hasSNI := anytlsNoSNI["sni"]; hasSNI {
		t.Fatalf("expected nil serverName not to emit sni:null, got %#v", anytlsNoSNI["sni"])
	}

	vlessReality := aerionClientNode(p.Record{
		"name":              "VLESS-REALITY",
		"protocol":          "vless",
		"host":              "edge.example.com",
		"port":              443.0,
		"tls":               true,
		"tlsMode":           "reality",
		"serverName":        "huggingface.co",
		"realitySettings":   map[string]any{"public_key": "pub-key", "short_id": "0102"},
		"transport":         "httpupgrade",
		"transportSettings": map[string]any{"path": "/up", "host": "cdn.example.com"},
		"protocolOptions":   map[string]any{"flow": "xtls-rprx-vision", "client_fingerprint": "chrome"},
	}, "secret", "uuid-1")
	vlessTLS, ok := vlessReality["tls"].(p.Record)
	if !ok || vlessTLS["enabled"] != true || vlessTLS["server_name"] != "huggingface.co" {
		t.Fatalf("unexpected VLESS tls object: %#v", vlessReality["tls"])
	}
	ro, ok := vlessReality["reality_opts"].(p.Record)
	if !ok || ro["public_key"] != "pub-key" || ro["short_id"] != "0102" {
		t.Fatalf("unexpected VLESS reality_opts: %#v", vlessReality["reality_opts"])
	}
	tr, ok := vlessReality["transport"].(p.Record)
	if !ok || tr["type"] != "httpupgrade" || tr["path"] != "/up" || tr["host"] != "cdn.example.com" {
		t.Fatalf("unexpected VLESS transport object: %#v", vlessReality["transport"])
	}
	if vlessReality["client_fingerprint"] != "chrome" {
		t.Fatalf("missing client_fingerprint: %#v", vlessReality)
	}

	plainVless := aerionClientNode(p.Record{
		"name":     "VLESS-Plain",
		"protocol": "vless",
		"host":     "plain.example.com",
		"port":     80.0,
		"tls":      false,
	}, "secret", "uuid-1")
	plainTLS, ok := plainVless["tls"].(p.Record)
	if !ok || plainTLS["enabled"] != false {
		t.Fatalf("expected plain VLESS tls.enabled=false, got %#v", plainVless["tls"])
	}

	hy2 := aerionClientNode(p.Record{
		"name":            "HY2",
		"protocol":        "hysteria2",
		"host":            "hy2.example.com",
		"port":            443.0,
		"tls":             true,
		"serverName":      "hy2.example.com",
		"protocolOptions": map[string]any{"obfs": "salamander", "obfs_password": "obfs-secret"},
	}, "secret", "uuid-1")
	if hy2["obfs_password"] != "obfs-secret" {
		t.Fatalf("missing obfs_password: %#v", hy2)
	}

	tuic := aerionClientNode(p.Record{
		"name":            "TUIC",
		"protocol":        "tuic",
		"host":            "tuic.example.com",
		"port":            443.0,
		"tls":             true,
		"serverName":      "tuic.example.com",
		"protocolOptions": map[string]any{"congestion_control": "bbr", "udp_relay_mode": "quic"},
	}, "uuid-1", "uuid-1")
	if tuic["congestion_control"] != "bbr" || tuic["udp_relay_mode"] != "quic" {
		t.Fatalf("missing TUIC snake_case fields: %#v", tuic)
	}

	sudokuNode := p.Record{
		"name":            "Sudoku",
		"protocol":        "sudoku",
		"host":            "sudoku.example.com",
		"port":            443.0,
		"protocolOptions": map[string]any{"aead": "aes-128-gcm", "http_mask": true, "http_mask_mode": "ws", "multiplex": "on", "path_root": "edge"},
	}
	req := buildAerionCapabilityRequest("secapi.telecom.moe", []string{"anytls", "sudoku", "naive"})
	if !supportedFormat(sudokuNode, "aerion", req) {
		t.Fatal("expected Sudoku to be supported in aerion format")
	}
	sudokuOut := aerionClientNode(sudokuNode, "uuid-1", "uuid-1")
	if sudokuOut["type"] != "sudoku" || sudokuOut["key"] != "uuid-1" || sudokuOut["http-mask-mode"] != "ws" || sudokuOut["http-mask-multiplex"] != "on" {
		t.Fatalf("unexpected Sudoku client node: %#v", sudokuOut)
	}

	naiveNode := p.Record{
		"name":       "Naive",
		"protocol":   "naive",
		"host":       "naive.example.com",
		"port":       443.0,
		"tls":        true,
		"serverName": "naive.example.com",
		"transport":  "quic",
	}
	if !supportedFormat(naiveNode, "aerion", req) || supportedFormat(naiveNode, "clash", req) {
		t.Fatal("Naive should only be supported in aerion format")
	}
	naiveOut := aerionClientNode(naiveNode, "proxy-pass", "uuid-1")
	naiveTLS, ok := naiveOut["tls"].(p.Record)
	if !ok || naiveTLS["enabled"] != true || naiveOut["quic"] != true {
		t.Fatalf("unexpected Naive client node: %#v", naiveOut)
	}

	migratedAcct := p.Record{"credential": "uuid-cred", "proxyPassword": "legacy-sspanel-pass"}
	if got := subscriptionPassword(migratedAcct, p.Record{"protocol": "anytls"}); got != "uuid-cred" {
		t.Fatalf("noders AnyTLS must use credential UUID, got %q", got)
	}
}

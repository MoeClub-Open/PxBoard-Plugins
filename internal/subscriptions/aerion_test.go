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

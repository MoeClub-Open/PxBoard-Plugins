package subscriptions

import (
	"net/http/httptest"
	"reflect"
	"testing"

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

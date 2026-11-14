package subscriptions

import (
	"net/http/httptest"
	"strings"
	"testing"

	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestFluxCapabilityGatingAndClashExport(t *testing.T) {
	node := p.Record{
		"name":     "JP-Flux",
		"protocol": "flux",
		"host":     "jp.example.com",
		"port":     443.0,
		"protocolOptions": map[string]any{
			"security":            "reality",
			"ipStrategy":          "dual",
			"standbyChannels":     2,
			"rotationIntervalSec": 180,
			"rotationBytesMb":     96,
			"clientFingerprint":   "chrome",
			"reality": map[string]any{
				"serverName": "huggingface.co",
				"publicKey":  "hSDwCYkwp1R0i33ctD73Wg2_Og0mOBr066SpjqqbTmo",
				"shortId":    "01234567",
			},
			"channels": []any{
				map[string]any{
					"id":           "direct-udp",
					"enabled":      true,
					"connectPorts": "443",
					"ipVersion":    "auto",
					"transport":    "udp",
					"role":         "primary",
					"weight":       100,
				},
				map[string]any{
					"id":           "cdn-ws",
					"enabled":      true,
					"connectHost":  "cdn.example.com",
					"connectPorts": "443",
					"ipVersion":    "ipv6",
					"transport":    "http",
					"httpMode":     "ws",
					"httpHost":     "cdn.example.com",
					"httpPath":     "/ws",
					"httpHeaders":  map[string]string{"X-Route": "edge"},
					"security":     "tls",
					"sni":          "cdn.example.com",
					"role":         "backup",
					"weight":       60,
				},
				map[string]any{
					"id":           "port-hop",
					"enabled":      true,
					"connectPorts": "20000-20010",
					"transport":    "udp",
					"security":     "obfs",
					"obfsKey":      "0123456789abcdef0123456789abcdef",
				},
			},
		},
	}

	ordinaryReq := httptest.NewRequest("GET", "/s/token?format=clash", nil)
	ordinaryReq.Header.Set("User-Agent", "Clash.Meta/v1.18.0")
	if supportedFormat(node, "clash", ordinaryReq) {
		t.Fatal("ordinary third-party Clash client should skip flux nodes")
	}

	typesReq := httptest.NewRequest("GET", "/s/token?format=aerion&types=vless,vmess,flux", nil)
	if !supportedFormat(node, "aerion", typesReq) {
		t.Fatal("Aerion adapter should export declared Flux nodes")
	}
	if supportedFormat(node, "clash", typesReq) || supportedFormat(node, "sing-box", typesReq) || supportedFormat(node, "uri", typesReq) {
		t.Fatal("Flux must only be emitted by the Aerion adapter")
	}

	headerReq := httptest.NewRequest("GET", "/s/token", nil)
	headerReq.Header.Set("X-XBClient-Protocols", "sudoku,flux")
	if !supportedFormat(node, "aerion", headerReq) {
		t.Fatal("client with X-XBClient-Protocols: sudoku,flux should receive flux")
	}

	uaReq := httptest.NewRequest("GET", "/s/token", nil)
	uaReq.Header.Set("User-Agent", "Aerion/0.3.0")
	if !supportedFormat(node, subscriptionFormat(uaReq), uaReq) {
		t.Fatal("Aerion User-Agent should receive flux")
	}

	netoUaReq := httptest.NewRequest("GET", "/s/token", nil)
	netoUaReq.Header.Set("User-Agent", "Neto/0.1.0")
	if subscriptionFormat(netoUaReq) != "aerion" || !supportedFormat(node, subscriptionFormat(netoUaReq), netoUaReq) {
		t.Fatal("Neto User-Agent should resolve to aerion format and receive flux")
	}

	netoQueryReq := httptest.NewRequest("GET", "/s/token?format=neto", nil)
	if subscriptionFormat(netoQueryReq) != "aerion" || !supportedFormat(node, subscriptionFormat(netoQueryReq), netoQueryReq) {
		t.Fatal("?format=neto should resolve to aerion format and receive flux")
	}

	legacyXbReq := httptest.NewRequest("GET", "/s/token?flag=clash&types=anytls,hysteria,trojan,vless,vmess,mieru,sudoku", nil)
	legacyXbReq.Header.Set("User-Agent", "Aerion")
	if supportedFormat(node, "clash", legacyXbReq) {
		t.Fatal("legacy XBClient with explicit types omitting flux must not receive flux")
	}

	proxy := aerion(node, "separate-proxy-password", "11111111-2222-3333-4444-555555555555")
	if proxy["type"] != "flux" || proxy["key"] != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected flux clash proxy identity: %#v", proxy)
	}
	for _, unexpected := range []string{"password", "uuid", "tls", "network", "sni"} {
		if _, exists := proxy[unexpected]; exists {
			t.Fatalf("unexpected extra top-level field %q on MihomoFluxProxy: %#v", unexpected, proxy)
		}
	}

	tmpl := "proxies: []\nproxy-groups:\n  - name: Proxy\n    type: select\n    proxies:\n      - " + sc.Nodes + "\nrules:\n  - MATCH,Proxy\n"
	rendered, err := sc.Render("clash", tmpl, []map[string]any{map[string]any(proxy)}, "TestSub", nil)
	if err != nil {
		t.Fatalf("sc.Render clash failed: %v", err)
	}
	yamlOut := string(rendered)
	for _, expected := range []string{"type: flux", "ip-strategy: dual", "public-key: hSDwCYkwp1R0i33ctD73Wg2_Og0mOBr066SpjqqbTmo", "http-mode: ws", "20000-20010"} {
		if !strings.Contains(yamlOut, expected) {
			t.Fatalf("rendered clash YAML missing %q:\n%s", expected, yamlOut)
		}
	}
}

func TestFluxAllTypesDoesNotClaimMihomoSupport(t *testing.T) {
	req := httptest.NewRequest("GET", "/s/token?format=clash&types=all", nil)
	req.Header.Set("User-Agent", "Mihomo/1.19.0")
	if supportsFlux(req) {
		t.Fatal("types=all does not make Mihomo support Flux")
	}
}

func TestFluxIndependentKeyInheritsNodeShortID(t *testing.T) {
	node := p.Record{"name": "Flux", "protocol": "flux", "host": "edge.example.com", "port": 443.0,
		"protocolOptions": map[string]any{"security": "reality", "reality": map[string]any{"publicKey": "node-public", "shortId": "01234567"},
			"channels": []any{map[string]any{"id": "independent", "transport": "tcp", "publicKey": "channel-public"}}}}
	result := aerion(node, "password", "user-id")
	channels := result["channels"].([]p.Record)
	reality := channels[0]["reality-opts"].(p.Record)
	if reality["short-id"] != "01234567" {
		t.Fatalf("missing inherited short ID: %v", reality)
	}
}

func TestFluxChannelCanEnableCertificateVerification(t *testing.T) {
	for _, key := range []string{"insecure", "allow_insecure", "skip-cert-verify"} {
		t.Run(key, func(t *testing.T) {
			node := p.Record{"name": "Flux", "protocol": "flux", "host": "edge.example.com", "port": 443.0,
				"tlsSettings": map[string]any{"allow_insecure": true},
				"protocolOptions": map[string]any{"security": "tls", "channels": []any{
					map[string]any{"id": "inherit", "transport": "tcp"},
					map[string]any{"id": "verified", "transport": "tcp", key: false},
					map[string]any{"id": "insecure", "transport": "tcp", key: true},
				}}}
			result := aerion(node, "password", "user-id")
			channels := result["channels"].([]p.Record)
			if result["skip-cert-verify"] != true {
				t.Fatal("missing node default")
			}
			if _, exists := channels[0]["skip-cert-verify"]; exists {
				t.Fatal("unset channel option must inherit")
			}
			if channels[1]["skip-cert-verify"] != false || channels[2]["skip-cert-verify"] != true {
				t.Fatal("explicit channel certificate verification setting was lost")
			}
		})
	}
}

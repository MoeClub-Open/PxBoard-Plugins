package subscriptionconfig

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultProfiles(t *testing.T) {
	for _, format := range []string{"clash", "sing-box"} {
		data, err := Render(format, Default(format), PreviewNodes(format), "Example", []string{"sub.example:443"})
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		var doc map[string]any
		if format == "clash" {
			err = yaml.Unmarshal(data, &doc)
		} else {
			err = json.Unmarshal(data, &doc)
		}
		if err != nil {
			t.Fatal(err)
		}
		if doc["dns"] == nil || strings.Contains(string(data), Nodes) {
			t.Fatal("profile missing DNS or left node marker")
		}
		if format == "clash" {
			groups, _ := objects(doc["proxy-groups"])
			refs, _ := stringsIn(groups[1]["proxies"])
			if len(refs) != 2 || !strings.Contains(string(data), "DOMAIN,sub.example,DIRECT") || doc["allow-lan"] != false {
				t.Fatal("invalid automatic group or host bypass")
			}
		} else {
			inbounds, _ := objects(doc["inbounds"])
			if len(inbounds) == 0 || object(doc, "route")["default_domain_resolver"] == nil {
				t.Fatal("missing inbound or bootstrap resolver")
			}
		}
	}
}

func TestFilterFallbackAndNameCollision(t *testing.T) {
	content := `proxies: []
proxy-groups:
  - {name: 香港, type: select, proxies: ["${nodes}"], include: "香港", exclude: "高倍率", fallback: DIRECT}
  - {name: 美国, type: select, proxies: ["${nodes}"], include: "美国", fallback: 香港}
rules: ["MATCH,香港"]
`
	nodes := PreviewNodes("clash")
	nodes = append(nodes, map[string]any{"name": "香港", "type": "trojan", "server": "node.example", "port": 443, "password": "sample"})
	data, err := Render("clash", content, nodes, "Site", nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = yaml.Unmarshal(data, &doc)
	groups, _ := objects(doc["proxy-groups"])
	refs, _ := stringsIn(groups[0]["proxies"])
	if len(refs) != 2 || refs[1] != "香港 · 2" {
		t.Fatal("filter or collision allocation failed", refs)
	}
	fallback, _ := stringsIn(groups[1]["proxies"])
	if len(fallback) != 1 || fallback[0] != "香港" {
		t.Fatal("fallback did not retain a group reference")
	}
	for _, invalid := range []string{strings.Replace(content, "include: \"香港\"", "include: \"(?=unsupported)\"", 1), strings.Replace(content, "fallback: 香港", "fallback: 美国", 1), strings.Replace(content, "MATCH,香港", "MATCH,missing", 1)} {
		if err := Check("clash", invalid); err == nil {
			t.Fatal("invalid filter, cycle or policy accepted")
		}
	}
}

func TestXboardConversion(t *testing.T) {
	legacy := `{"dns":{"servers":[{"tag":"remote","address":"https://1.1.1.1/dns-query","detour":"proxy"},{"tag":"local","address":"https://223.5.5.5/dns-query"}],"rules":[{"outbound":["any"],"server":"local"}]},"inbounds":[{"type":"mixed","listen":"127.0.0.1","listen_port":7890,"sniff":true,"domain_strategy":"prefer_ipv4"}],"outbounds":[{"type":"selector","tag":"proxy","outbounds":["auto"]},{"type":"urltest","tag":"auto","outbounds":[]},{"type":"direct","tag":"direct"},{"type":"dns","tag":"dns-out"}],"route":{"rules":[{"protocol":"dns","outbound":"dns-out"}]}}`
	format, converted, err := Xboard("singbox", legacy)
	if err != nil || format != "sing-box" {
		t.Fatal(err)
	}
	data, err := Render(format, converted, PreviewNodes(format), "Site", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"address": "https`) || strings.Contains(string(data), `"type": "dns"`) || !strings.Contains(string(data), "hijack-dns") || !strings.Contains(string(data), "default_domain_resolver") {
		t.Fatal("legacy fields not upgraded")
	}
	clash := `proxies: []
proxy-groups:
  - {name: "$app_name", type: select, proxies: []}
  - {name: HK, type: select, proxies: ["/香港/i"]}
rules: ["MATCH,$app_name"]
`
	_, converted, err = Xboard("clashmeta", clash)
	if err != nil {
		t.Fatal(err)
	}
	data, err = Render("clash", converted, PreviewNodes("clash"), `Site: "quoted"`, nil)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal("title damaged YAML", err)
	}
}

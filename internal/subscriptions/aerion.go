package subscriptions

import (
	"fmt"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"gopkg.in/yaml.v3"
)

func naiveAerion(n p.Record, cred string) p.Record {
	out := p.Record{
		"name":     n["name"],
		"type":     "naive",
		"server":   n["host"],
		"port":     n["port"],
		"password": cred,
	}
	options := object(n, "protocolOptions")
	transport := strings.ToLower(strings.TrimSpace(p.Text(n, "transport")))
	if transport == "quic" || transport == "h3" || transport == "http3" || p.Bool(options, "quic") {
		out["quic"] = true
	}
	if cc := strings.TrimSpace(p.Text(options, "congestion_control")); cc != "" {
		out["quic_congestion_control"] = cc
	} else if cc := strings.TrimSpace(p.Text(options, "quic_congestion_control")); cc != "" {
		out["quic_congestion_control"] = cc
	}
	if hdrs := object(options, "extra_headers"); len(hdrs) > 0 {
		out["extra_headers"] = hdrs
	}
	clashSecurity(n, out)
	return out
}

func aerion(n p.Record, cred, uuid string) p.Record {
	switch p.Text(n, "protocol") {
	case "flux":
		return fluxClash(n, cred, uuid)
	case "naive":
		return naiveAerion(n, cred)
	}
	return clash(n, cred, uuid)
}

func anyMapRecord(v any) p.Record {
	switch m := v.(type) {
	case p.Record:
		return m
	case map[string]any:
		return p.Record(m)
	default:
		return nil
	}
}

func aerionClientNode(n p.Record, cred, uuid string) p.Record {
	proxy := aerion(n, cred, uuid)
	out := make(p.Record, len(proxy)+6)
	for k, v := range proxy {
		if v == nil && (k == "sni" || k == "servername") {
			continue
		}
		out[k] = v
	}
	if server, ok := out["server"]; ok {
		out["host"] = server
	}
	skip := p.Bool(out, "skip-cert-verify") || p.Bool(out, "insecure")
	if _, hasSkip := out["skip-cert-verify"]; hasSkip || skip {
		out["insecure"] = skip
	}
	if rawTLS, hasTLS := out["tls"]; hasTLS {
		if tlsEnabled, isBool := rawTLS.(bool); isBool {
			tlsObj := p.Record{"enabled": tlsEnabled}
			if tlsEnabled {
				sni := strings.TrimSpace(p.Text(out, "sni"))
				if sni == "" {
					sni = strings.TrimSpace(p.Text(out, "servername"))
				}
				if sni == "" {
					sni = strings.TrimSpace(p.Text(n, "serverName"))
				}
				if sni != "" {
					tlsObj["server_name"] = sni
					out["sni"] = sni
				}
				if skip {
					tlsObj["insecure"] = true
				}
				if alpn, ok := out["alpn"]; ok && alpn != nil {
					tlsObj["alpn"] = alpn
				}
				if ro := anyMapRecord(out["reality-opts"]); ro != nil {
					pubKey, shortID := p.Text(ro, "public-key"), p.Text(ro, "short-id")
					tlsObj["reality"] = p.Record{"enabled": true, "public_key": pubKey, "short_id": shortID}
					out["reality_opts"] = p.Record{"public_key": pubKey, "short_id": shortID}
				}
			}
			out["tls"] = tlsObj
		}
	}
	if fp := strings.TrimSpace(p.Text(out, "client-fingerprint")); fp != "" {
		out["client_fingerprint"] = fp
	}
	if streamProtocol(n) && network(n) != "tcp" {
		kind, settings := network(n), object(n, "transportSettings")
		tr := p.Record{"type": kind}
		switch kind {
		case "grpc":
			if sn := strings.TrimSpace(p.Text(settings, "serviceName")); sn != "" {
				tr["service_name"] = sn
			}
		case "ws", "httpupgrade":
			if path := strings.TrimSpace(p.Text(settings, "path")); path != "" {
				tr["path"] = path
			}
			if host := strings.TrimSpace(transportHost(settings)); host != "" {
				tr["host"] = host
			}
			if hdrs := object(settings, "headers"); len(hdrs) > 0 {
				tr["headers"] = hdrs
			}
		case "h2":
			if path := strings.TrimSpace(p.Text(settings, "path")); path != "" {
				tr["path"] = path
			}
			if host := strings.TrimSpace(transportHost(settings)); host != "" {
				tr["host"] = host
			}
		case "xhttp":
			if path := strings.TrimSpace(p.Text(settings, "path")); path != "" {
				tr["path"] = path
			}
			if host := strings.TrimSpace(transportHost(settings)); host != "" {
				tr["host"] = host
			}
			if mode := strings.TrimSpace(p.Text(settings, "mode")); mode != "" {
				tr["mode"] = mode
			}
			if hdrs := object(settings, "headers"); len(hdrs) > 0 {
				tr["headers"] = hdrs
			}
		}
		out["transport"] = tr
	}
	if op := p.Text(out, "obfs-password"); op != "" {
		out["obfs_password"] = op
	}
	if cc := p.Text(out, "congestion-controller"); cc != "" {
		out["congestion_control"] = cc
	}
	if urm := p.Text(out, "udp-relay-mode"); urm != "" {
		out["udp_relay_mode"] = urm
	}
	if tp := p.Text(out, "traffic-pattern"); tp != "" {
		out["traffic_pattern"] = tp
	}
	if aid, ok := out["alterId"]; ok {
		out["alter_id"] = aid
	}
	return out
}

type aerionRouting struct {
	HasRules          bool     `json:"hasRules"`
	RuleCount         int      `json:"ruleCount"`
	ProxyGroupCount   int      `json:"proxyGroupCount"`
	RuleProviderCount int      `json:"ruleProviderCount"`
	Rules             []string `json:"rules"`
	RulesPreview      []string `json:"rulesPreview"`
	RouteConfigYAML   string   `json:"routeConfigYaml"`
}

func emptyAerionRouting() aerionRouting {
	return aerionRouting{
		Rules:        []string{},
		RulesPreview: []string{},
	}
}

func sanitizeAerionYAML(raw []byte) ([]byte, aerionRouting, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, emptyAerionRouting(), err
	}
	if doc == nil {
		return raw, emptyAerionRouting(), nil
	}
	stripped := map[string]bool{}
	keptProviders := map[string]any{}
	for _, providerKey := range []string{"rule-providers", "rule_providers"} {
		rawProviders, exists := doc[providerKey]
		if !exists || rawProviders == nil {
			continue
		}
		providers, ok := rawProviders.(map[string]any)
		if !ok {
			delete(doc, providerKey)
			continue
		}
		for name, item := range providers {
			if isSupportedInlineRuleProvider(item) {
				keptProviders[name] = item
			} else {
				stripped[name] = true
			}
		}
		delete(doc, providerKey)
	}
	if len(keptProviders) > 0 {
		doc["rule-providers"] = keptProviders
	}

	if dns, ok := doc["dns"].(map[string]any); ok {
		if policy, ok := dns["nameserver-policy"].(map[string]any); ok {
			for k := range policy {
				lower := strings.ToLower(strings.TrimSpace(k))
				if strings.HasPrefix(lower, "rule-set:") {
					setName := strings.TrimSpace(k[len("rule-set:"):])
					if stripped[setName] {
						delete(policy, k)
					}
				}
			}
			if len(policy) == 0 {
				delete(dns, "nameserver-policy")
			}
		}
	}

	rawRules, _ := doc["rules"].([]any)
	hasCNDomain := false
	for _, item := range rawRules {
		line := strings.TrimSpace(fmt.Sprint(item))
		if strings.HasPrefix(strings.ToUpper(line), "DOMAIN-SUFFIX,CN,") {
			hasCNDomain = true
			break
		}
	}
	cleanedRules := make([]string, 0, len(rawRules))
	cleanedAny := make([]any, 0, len(rawRules))
	for _, item := range rawRules {
		line, ok := item.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Split(trimmed, ",")
		if len(parts) >= 3 && strings.EqualFold(strings.TrimSpace(parts[0]), "RULE-SET") {
			setName := strings.TrimSpace(parts[1])
			target := strings.TrimSpace(parts[2])
			if stripped[setName] {
				lowerName := strings.ToLower(setName)
				if !hasCNDomain && (lowerName == "cn-domain" || (strings.Contains(lowerName, "cn") && strings.Contains(lowerName, "domain"))) {
					replacement := "DOMAIN-SUFFIX,cn," + target
					cleanedRules = append(cleanedRules, replacement)
					cleanedAny = append(cleanedAny, replacement)
					hasCNDomain = true
				}
				continue
			}
		}
		cleanedRules = append(cleanedRules, trimmed)
		cleanedAny = append(cleanedAny, trimmed)
	}
	doc["rules"] = cleanedAny

	sanitized, err := yaml.Marshal(doc)
	if err != nil {
		return nil, emptyAerionRouting(), err
	}
	proxyGroupCount := 0
	if groups, ok := doc["proxy-groups"].([]any); ok {
		proxyGroupCount = len(groups)
	}
	previewLen := min(20, len(cleanedRules))
	preview := make([]string, previewLen)
	copy(preview, cleanedRules[:previewLen])
	routeYAML := ""
	if len(cleanedRules) > 0 {
		routeYAML = string(sanitized)
	}
	return sanitized, aerionRouting{
		HasRules:          len(cleanedRules) > 0,
		RuleCount:         len(cleanedRules),
		ProxyGroupCount:   proxyGroupCount,
		RuleProviderCount: len(keptProviders),
		Rules:             cleanedRules,
		RulesPreview:      preview,
		RouteConfigYAML:   routeYAML,
	}, nil
}

func isSupportedInlineRuleProvider(raw any) bool {
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(m["type"])), "inline") {
		return false
	}
	if strings.TrimSpace(fmt.Sprint(m["behavior"])) == "" || m["behavior"] == nil {
		return false
	}
	payload, ok := m["payload"].([]any)
	if !ok || len(payload) == 0 {
		return false
	}
	for _, unsupported := range []string{"path", "url", "format"} {
		if _, exists := m[unsupported]; exists {
			return false
		}
	}
	return true
}

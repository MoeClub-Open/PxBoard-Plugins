package subscriptions

import (
	"fmt"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"gopkg.in/yaml.v3"
)

func aerion(n p.Record, cred, uuid string) p.Record {
	if p.Text(n, "protocol") == "flux" {
		return fluxClash(n, cred, uuid)
	}
	return clash(n, cred, uuid)
}

func aerionClientNode(n p.Record, cred, uuid string) p.Record {
	proxy := aerion(n, cred, uuid)
	out := make(p.Record, len(proxy)+2)
	for k, v := range proxy {
		out[k] = v
	}
	if server, ok := out["server"]; ok {
		out["host"] = server
	}
	if skip, ok := out["skip-cert-verify"]; ok {
		out["insecure"] = skip
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

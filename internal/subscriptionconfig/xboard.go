package subscriptionconfig

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Xboard converts stored template data once. Runtime rendering uses only the
// current format; it does not switch to legacy client configuration schemas.
func Xboard(name, content string) (string, string, error) {
	format := "clash"
	switch strings.ToLower(name) {
	case "clash", "clashmeta":
	case "singbox", "sing-box":
		format = "sing-box"
	default:
		return "", "", fmt.Errorf("未支持的模板格式：%s", name)
	}
	config, err := parse(format, content)
	if err != nil {
		return format, "", err
	}
	if format == "clash" {
		groups, err := objects(config["proxy-groups"])
		if err != nil {
			return format, "", err
		}
		for _, group := range groups {
			values, err := stringsIn(group["proxies"])
			if err != nil {
				return format, "", err
			}
			refs := []any{}
			patterns := []string{}
			for _, value := range values {
				if len(value) > 2 && strings.ContainsRune("/#~@%", rune(value[0])) && strings.LastIndexByte(value[1:], value[0]) >= 0 {
					pattern, err := Pattern(value)
					if err != nil {
						return format, "", err
					}
					patterns = append(patterns, "(?:"+pattern.String()+")")
				} else {
					refs = append(refs, value)
				}
			}
			refs = append(refs, Nodes)
			group["proxies"] = refs
			if len(patterns) > 0 {
				group["include"] = strings.Join(patterns, "|")
			}
			if group["fallback"] == nil {
				group["fallback"] = []any{"自动选择", "DIRECT"}
			}
		}
		contentBytes, err := Encode(format, config)
		if err != nil {
			return format, "", err
		}
		content = strings.ReplaceAll(string(contentBytes), "$app_name", "${siteName}")
	} else {
		if err := upgradeSingbox(config); err != nil {
			return format, "", err
		}
		encoded, err := Encode(format, config)
		if err != nil {
			return format, "", err
		}
		content = string(encoded)
	}
	if err := Check(format, content); err != nil {
		return format, content, err
	}
	return format, content, nil
}

func upgradeSingbox(config map[string]any) error {
	route := object(config, "route")
	if route == nil {
		route = map[string]any{}
		config["route"] = route
	}
	rows, err := objects(config["outbounds"])
	if err != nil {
		return err
	}
	kept := []map[string]any{}
	removed := map[string]string{}
	selector := ""
	for _, row := range rows {
		typ, tag := text(row, "type"), text(row, "tag")
		if typ == "dns" || typ == "block" {
			removed[tag] = typ
			continue
		}
		if typ == "selector" || typ == "urltest" {
			if selector == "" {
				selector = tag
			}
			refs, err := stringsIn(row["outbounds"])
			if err != nil {
				return err
			}
			values := []any{}
			for _, ref := range refs {
				values = append(values, ref)
			}
			row["outbounds"] = append(values, Nodes)
			if row["fallback"] == nil {
				row["fallback"] = "direct"
			}
		}
		kept = append(kept, row)
	}
	config["outbounds"] = rowsAsList(kept)
	if text(route, "final") == "" {
		route["final"] = selector
	}
	if removed[text(route, "final")] != "" {
		return fmt.Errorf("默认路由指向已移除的特殊出站，请人工调整")
	}
	var upgradeRules func(any) error
	upgradeRules = func(value any) error {
		rules, err := objects(value)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if typ := removed[text(rule, "outbound")]; typ != "" {
				delete(rule, "outbound")
				if typ == "dns" {
					rule["action"] = "hijack-dns"
				} else {
					rule["action"] = "reject"
				}
			}
			if rule["rules"] != nil {
				if err := upgradeRules(rule["rules"]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := upgradeRules(route["rules"]); err != nil {
		return err
	}
	inbounds, err := objects(config["inbounds"])
	if err != nil {
		return err
	}
	sniff := false
	for _, inbound := range inbounds {
		sniff = sniff || inbound["sniff"] == true
		if inbound["address"] == nil {
			values := []any{}
			for _, key := range []string{"inet4_address", "inet6_address"} {
				if value := inbound[key]; value != nil {
					switch value := value.(type) {
					case string:
						values = append(values, value)
					case []any:
						values = append(values, value...)
					}
				}
			}
			if len(values) > 0 {
				inbound["address"] = values
			}
		}
		for _, key := range []string{"sniff", "sniff_timeout", "sniff_override_destination", "domain_strategy", "endpoint_independent_nat", "inet4_address", "inet6_address"} {
			delete(inbound, key)
		}
	}
	if sniff {
		rules, _ := route["rules"].([]any)
		route["rules"] = append([]any{map[string]any{"action": "sniff"}}, rules...)
	}
	return upgradeDNS(config, route)
}

func upgradeDNS(config, route map[string]any) error {
	dns := object(config, "dns")
	if dns == nil {
		return nil
	}
	rows, err := objects(dns["servers"])
	if err != nil {
		return err
	}
	kept := []map[string]any{}
	rcodes := map[string]string{}
	first := ""
	for _, server := range rows {
		address := text(server, "address")
		tag := text(server, "tag")
		if strings.HasPrefix(address, "rcode://") {
			code := map[string]string{"success": "NOERROR", "name_error": "NXDOMAIN", "refused": "REFUSED", "server_failure": "SERVFAIL"}[strings.TrimPrefix(address, "rcode://")]
			if code == "" {
				return fmt.Errorf("不支持的旧 DNS 响应类型")
			}
			rcodes[tag] = code
			continue
		}
		if address != "" {
			delete(server, "address")
			if address == "local" {
				server["type"] = "local"
			} else {
				if !strings.Contains(address, "://") {
					address = "udp://" + address
				}
				u, err := url.Parse(address)
				if err != nil || u.Hostname() == "" {
					return fmt.Errorf("旧 DNS 地址无效")
				}
				switch u.Scheme {
				case "udp", "tcp", "tls", "https", "quic", "h3":
					server["type"] = u.Scheme
				default:
					return fmt.Errorf("旧 DNS 协议无法自动转换：%s", u.Scheme)
				}
				server["server"] = u.Hostname()
				if port := u.Port(); port != "" {
					value, err := strconv.Atoi(port)
					if err != nil {
						return err
					}
					server["server_port"] = value
				}
				if u.Path != "" {
					server["path"] = u.Path
				}
			}
		}
		if resolver := server["address_resolver"]; resolver != nil {
			server["domain_resolver"] = resolver
			delete(server, "address_resolver")
		}
		if first == "" {
			first = tag
		}
		kept = append(kept, server)
	}
	dns["servers"] = rowsAsList(kept)
	if text(dns, "final") == "" {
		dns["final"] = first
	}
	rules, err := objects(dns["rules"])
	if err != nil {
		return err
	}
	next := []any{}
	for _, rule := range rules {
		if outbound, exists := rule["outbound"]; exists {
			anyOutbound := outbound == "any"
			if list, ok := outbound.([]any); ok {
				anyOutbound = len(list) == 1 && list[0] == "any"
			}
			if !anyOutbound || len(rule) != 2 {
				return fmt.Errorf("旧 DNS outbound 匹配需要人工转换")
			}
			if route["default_domain_resolver"] == nil {
				route["default_domain_resolver"] = rule["server"]
			}
			continue
		}
		if code := rcodes[text(rule, "server")]; code != "" {
			delete(rule, "server")
			rule["action"] = "predefined"
			rule["rcode"] = code
		}
		next = append(next, rule)
	}
	dns["rules"] = next
	return nil
}

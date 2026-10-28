package subscriptionconfig

import (
	"fmt"
	"strings"
)

func validate(format string, config map[string]any) error {
	if format == "clash" {
		return validateClash(config)
	}
	return validateSingbox(config)
}

func validateClash(config map[string]any) error {
	nodes, err := objects(config["proxies"])
	if err != nil {
		return err
	}
	groups, err := objects(config["proxy-groups"])
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		return fmt.Errorf("模板缺少 proxy-groups")
	}
	known := map[string]bool{"DIRECT": true, "REJECT": true, "REJECT-DROP": true, "PASS": true, "COMPATIBLE": true}
	for _, node := range append(nodes, groups...) {
		key := text(node, "name")
		if key == "" || known[key] {
			return fmt.Errorf("名称缺失或重复：%s", key)
		}
		known[key] = true
	}
	edges := map[string][]string{}
	for _, group := range groups {
		refs, err := stringsIn(group["proxies"])
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if !known[ref] {
				return fmt.Errorf("策略组 %s 引用了不存在的名称：%s", text(group, "name"), ref)
			}
		}
		edges[text(group, "name")] = refs
	}
	if err := checkCycles(edges); err != nil {
		return err
	}
	rules, err := stringsIn(config["rules"])
	if err != nil || len(rules) == 0 {
		return fmt.Errorf("模板缺少有效的 rules")
	}
	providers := object(config, "rule-providers")
	for _, rule := range rules {
		parts := strings.Split(rule, ",")
		if len(parts) < 2 {
			return fmt.Errorf("分流规则无效：%s", rule)
		}
		last := len(parts) - 1
		if parts[last] == "no-resolve" {
			last--
		}
		if last < 1 {
			return fmt.Errorf("分流规则缺少策略")
		}
		if !known[strings.TrimSpace(parts[last])] {
			return fmt.Errorf("分流规则引用了不存在的策略：%s", parts[last])
		}
		if parts[0] == "RULE-SET" && (len(parts) < 3 || providers[parts[1]] == nil) {
			return fmt.Errorf("分流规则引用了不存在的规则集")
		}
	}
	return nil
}

func validateSingbox(config map[string]any) error {
	rows, err := objects(config["outbounds"])
	if err != nil {
		return err
	}
	known := map[string]bool{}
	edges := map[string][]string{}
	for _, row := range rows {
		tag := text(row, "tag")
		if tag == "" || known[tag] {
			return fmt.Errorf("出站名称缺失或重复：%s", tag)
		}
		known[tag] = true
		if text(row, "type") == "block" || text(row, "type") == "dns" {
			return fmt.Errorf("旧版 block/dns 出站需要先转换为路由 action")
		}
	}
	for _, row := range rows {
		if text(row, "type") == "selector" || text(row, "type") == "urltest" {
			refs, err := stringsIn(row["outbounds"])
			if err != nil {
				return err
			}
			for _, ref := range refs {
				if !known[ref] {
					return fmt.Errorf("出站组引用不存在：%s", ref)
				}
			}
			edges[text(row, "tag")] = refs
			if def := text(row, "default"); def != "" {
				found := false
				for _, ref := range refs {
					found = found || ref == def
				}
				if !found {
					return fmt.Errorf("出站组默认项不在组内：%s", def)
				}
			}
		}
		if detour := text(row, "detour"); detour != "" && !known[detour] {
			return fmt.Errorf("出站 detour 引用不存在：%s", detour)
		}
	}
	if err := checkCycles(edges); err != nil {
		return err
	}
	route := object(config, "route")
	if route == nil || !known[text(route, "final")] {
		return fmt.Errorf("route.final 必须指向有效的出站或策略组")
	}
	ruleSets, err := objects(route["rule_set"])
	if err != nil {
		return err
	}
	sets := map[string]bool{}
	for _, set := range ruleSets {
		tag := text(set, "tag")
		if tag == "" || sets[tag] {
			return fmt.Errorf("规则集标签缺失或重复")
		}
		sets[tag] = true
		if detour := text(set, "download_detour"); detour != "" && !known[detour] {
			return fmt.Errorf("规则集下载出站不存在")
		}
	}
	if err := checkRules(route["rules"], known, sets, false, nil); err != nil {
		return err
	}
	dns := object(config, "dns")
	if dns != nil {
		servers, err := objects(dns["servers"])
		if err != nil {
			return err
		}
		tags := map[string]bool{}
		for _, server := range servers {
			if text(server, "address") != "" || text(server, "type") == "" {
				return fmt.Errorf("旧版 DNS address 需要转换为带 type 的 DNS 服务器")
			}
			tag := text(server, "tag")
			if tag == "" || tags[tag] {
				return fmt.Errorf("DNS 标签缺失或重复")
			}
			tags[tag] = true
			if detour := text(server, "detour"); detour != "" && !known[detour] {
				return fmt.Errorf("DNS detour 引用不存在")
			}
		}
		if final := text(dns, "final"); final != "" && !tags[final] {
			return fmt.Errorf("DNS 默认服务器不存在")
		}
		if err := checkRules(dns["rules"], known, sets, true, tags); err != nil {
			return err
		}
		resolver := route["default_domain_resolver"]
		if value, ok := resolver.(string); ok && !tags[value] {
			return fmt.Errorf("默认域名解析器不存在")
		}
	}
	inbounds, err := objects(config["inbounds"])
	if err != nil {
		return err
	}
	for _, inbound := range inbounds {
		for _, key := range []string{"sniff", "sniff_override_destination", "domain_strategy", "endpoint_independent_nat", "inet4_address", "inet6_address"} {
			if _, exists := inbound[key]; exists {
				return fmt.Errorf("旧入站字段 %s 需要转换为当前配置", key)
			}
		}
	}
	return nil
}

func checkRules(value any, outbounds, sets map[string]bool, dns bool, servers map[string]bool) error {
	rules, err := objects(value)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if tag := text(rule, "outbound"); tag != "" && !outbounds[tag] {
			return fmt.Errorf("路由出站不存在：%s", tag)
		}
		if dns {
			if _, exists := rule["outbound"]; exists {
				return fmt.Errorf("旧 DNS outbound 匹配需要迁移")
			}
			if tag := text(rule, "server"); tag != "" && !servers[tag] {
				return fmt.Errorf("DNS 规则引用不存在：%s", tag)
			}
		}
		if values, exists := rule["rule_set"]; exists {
			var names []string
			if name, ok := values.(string); ok {
				names = []string{name}
			} else {
				names, err = stringsIn(values)
				if err != nil {
					return err
				}
			}
			for _, name := range names {
				if !sets[name] {
					return fmt.Errorf("规则集引用不存在：%s", name)
				}
			}
		}
		if rule["rules"] != nil {
			if err := checkRules(rule["rules"], outbounds, sets, dns, servers); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkCycles(edges map[string][]string) error {
	state := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if state[name] == 1 {
			return fmt.Errorf("策略组存在循环引用：%s", name)
		}
		if state[name] == 2 {
			return nil
		}
		state[name] = 1
		for _, next := range edges[name] {
			if _, group := edges[next]; group {
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		state[name] = 2
		return nil
	}
	for name := range edges {
		if err := visit(name); err != nil {
			return err
		}
	}
	return nil
}

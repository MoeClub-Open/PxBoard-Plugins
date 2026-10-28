package subscriptionconfig

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

func Pattern(value string) (*regexp.Regexp, error) {
	value = strings.TrimSpace(value)
	if len(value) > 2 && strings.ContainsRune("/#~@%", rune(value[0])) {
		if end := strings.LastIndexByte(value[1:], value[0]); end >= 0 {
			end++
			flags := value[end+1:]
			body := value[1:end]
			for _, flag := range flags {
				if flag != 'i' && flag != 'u' && flag != 'm' && flag != 's' {
					return nil, fmt.Errorf("不支持的正则标志")
				}
			}
			flags = strings.ReplaceAll(flags, "u", "")
			if flags != "" {
				body = "(?" + flags + ")" + body
			}
			value = body
		}
	}
	compiled, err := regexp.Compile(value)
	if err != nil {
		return nil, fmt.Errorf("节点筛选正则无效：%w", err)
	}
	return compiled, nil
}

func rowsAsList(rows []map[string]any) []any {
	list := make([]any, len(rows))
	for i, row := range rows {
		list[i] = row
	}
	return list
}

func Render(format, content string, nodes []map[string]any, title string, hosts []string) ([]byte, error) {
	config, err := parse(format, content)
	if err != nil {
		return nil, err
	}
	title = strings.NewReplacer(",", " ", "\r", " ", "\n", " ").Replace(title)
	replaceTitle(config, title)
	root, groups, name, refs := "proxies", "proxy-groups", "name", "proxies"
	if format == "sing-box" {
		root, groups, name, refs = "outbounds", "outbounds", "tag", "outbounds"
	}
	static, err := objects(config[root])
	if err != nil {
		return nil, err
	}
	selectors := static
	if format == "clash" {
		selectors, err = objects(config[groups])
		if err != nil {
			return nil, err
		}
	}
	known := map[string]bool{}
	if format == "clash" {
		for _, key := range []string{"DIRECT", "REJECT", "REJECT-DROP", "PASS", "COMPATIBLE"} {
			known[key] = true
		}
	}
	for _, row := range static {
		key := text(row, name)
		if key == "" || known[key] {
			return nil, fmt.Errorf("模板内名称缺失或重复：%s", key)
		}
		known[key] = true
	}
	if format == "clash" {
		for _, row := range selectors {
			key := text(row, name)
			if key == "" || known[key] {
				return nil, fmt.Errorf("策略组名称缺失或重复：%s", key)
			}
			known[key] = true
		}
	}
	generated := []map[string]any{}
	names := []string{}
	for _, original := range nodes {
		row := map[string]any{}
		for key, value := range original {
			row[key] = value
		}
		base := text(row, name)
		if base == "" {
			return nil, fmt.Errorf("节点名称不能为空")
		}
		key := base
		for suffix := 2; known[key]; suffix++ {
			key = fmt.Sprintf("%s · %d", base, suffix)
		}
		row[name] = key
		known[key] = true
		names = append(names, key)
		generated = append(generated, row)
	}
	for _, group := range selectors {
		if format == "sing-box" && text(group, "type") != "selector" && text(group, "type") != "urltest" {
			continue
		}
		if err := fillGroup(group, refs, name, names, known); err != nil {
			return nil, err
		}
	}
	config[root] = rowsAsList(append(static, generated...))
	if err := validate(format, config); err != nil {
		return nil, err
	}
	addDirectRules(format, config, hosts)
	data, err := Encode(format, config)
	if err != nil {
		return nil, err
	}
	if strings.Contains(string(data), Nodes) {
		return nil, fmt.Errorf("节点占位符只能用于策略组的名称列表")
	}
	return data, nil
}

func fillGroup(group map[string]any, refs, name string, names []string, known map[string]bool) error {
	matches := names
	for _, key := range []string{"include", "exclude"} {
		value := text(group, key)
		if value == "" {
			continue
		}
		pattern, err := Pattern(value)
		if err != nil {
			return fmt.Errorf("分组 %s：%w", text(group, name), err)
		}
		filtered := []string{}
		for _, node := range matches {
			if pattern.MatchString(node) == (key == "include") {
				filtered = append(filtered, node)
			}
		}
		matches = filtered
	}
	values, err := stringsIn(group[refs])
	if err != nil {
		return err
	}
	result := []any{}
	seen := map[string]bool{}
	add := func(value string) {
		if !seen[value] {
			result = append(result, value)
			seen[value] = true
		}
	}
	for _, value := range values {
		if value == Nodes {
			for _, node := range matches {
				add(node)
			}
		} else {
			add(value)
		}
	}
	if len(result) == 0 {
		var candidates []string
		if value, ok := group["fallback"].(string); ok {
			candidates = []string{value}
		} else {
			candidates, err = stringsIn(group["fallback"])
			if err != nil {
				return err
			}
		}
		for _, candidate := range candidates {
			if candidate == text(group, name) {
				continue
			}
			if known[candidate] {
				add(candidate)
				break
			}
			pattern, err := Pattern(candidate)
			if err != nil {
				return err
			}
			for _, node := range names {
				if pattern.MatchString(node) {
					add(node)
				}
			}
			if len(result) > 0 {
				break
			}
		}
	}
	if len(result) == 0 && group["include-all"] != true && group["include-all-proxies"] != true && group["include-all-providers"] != true && group["use"] == nil {
		return fmt.Errorf("分组 %s 没有节点，请配置有效的 fallback", text(group, name))
	}
	group[refs] = result
	for _, key := range []string{"include", "exclude", "fallback"} {
		delete(group, key)
	}
	return nil
}

func addDirectRules(format string, config map[string]any, hosts []string) {
	seen := map[string]bool{}
	domains := []string{}
	ips := []string{}
	for _, value := range hosts {
		if !strings.Contains(value, "://") {
			value = "https://" + value
		}
		u, err := url.Parse(value)
		if err != nil || u.User != nil {
			continue
		}
		host := strings.ToLower(u.Hostname())
		if host == "" || seen[host] || strings.ContainsAny(host, ", \r\n") {
			continue
		}
		seen[host] = true
		if ip := net.ParseIP(host); ip != nil {
			bits := 128
			if ip.To4() != nil {
				bits = 32
			}
			ips = append(ips, fmt.Sprintf("%s/%d", host, bits))
		} else {
			domains = append(domains, host)
		}
	}
	if format == "clash" {
		rules := []any{}
		for _, domain := range domains {
			rules = append(rules, "DOMAIN,"+domain+",DIRECT")
		}
		for _, ip := range ips {
			kind := "IP-CIDR"
			if strings.Contains(ip, ":") {
				kind = "IP-CIDR6"
			}
			rules = append(rules, kind+","+ip+",DIRECT,no-resolve")
		}
		old, _ := config["rules"].([]any)
		config["rules"] = append(rules, old...)
		return
	}
	rows, _ := objects(config["outbounds"])
	direct := ""
	for _, row := range rows {
		if text(row, "type") == "direct" {
			direct = text(row, "tag")
			break
		}
	}
	if direct == "" {
		return
	}
	route := object(config, "route")
	old, _ := route["rules"].([]any)
	rules := []any{}
	if len(domains) > 0 {
		rules = append(rules, map[string]any{"domain": domains, "outbound": direct})
	}
	if len(ips) > 0 {
		rules = append(rules, map[string]any{"ip_cidr": ips, "outbound": direct})
	}
	route["rules"] = append(rules, old...)
}

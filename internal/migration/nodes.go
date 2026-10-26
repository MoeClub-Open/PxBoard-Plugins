package migration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strconv"
	"strings"
)

func (m *mapper) newKey(label string) string {
	if m.Options.Seed == "" {
		m.fail("生成迁移机器密钥需要持久任务种子")
		return ""
	}
	mac := hmac.New(sha256.New, []byte(m.Options.Seed))
	_, _ = mac.Write([]byte(label))
	return hex.EncodeToString(mac.Sum(nil))
}
func (m *mapper) v2Nodes() {
	m.warning("节点记录已转换；服务器证书、私钥文件和外部服务不在迁移快照中，请在接入 NodeRS 前核对证书与监听设置。")
	table, rows := m.table("v2_server_machine")
	nextMachine := int64(0)
	for _, row := range rows {
		number := integer(row, "id")
		if number <= 0 || truth(row, "is_active") && text(row, "token") == "" {
			m.fail("机器 %s 的编号或接入密钥无效", text(row, "name"))
		}
		nextMachine = max(nextMachine, number)
		m.Machines[text(row, "id")] = true
		m.doc("nodes", "machines", m.id("machine", text(row, "id")), p.Record{"name": row["name"], "number": number, "key": row["token"], "enabled": truth(row, "is_active"), "createdAt": m.created(row, "created_at"), "source": m.ref(row, table)})
	}
	byHost := map[string]int64{}
	mapNode := func(row Row, table, protocol string, wireID int64) {
		host := text(row, "host")
		machine := integer(row, "machine_id")
		if machine > 0 && !m.Machines[strconv.FormatInt(machine, 10)] {
			m.fail("节点 %s 引用了不存在的机器 %d", text(row, "name"), machine)
		}
		if machine == 0 {
			machine = byHost[host]
			if machine == 0 {
				nextMachine++
				machine = nextMachine
				byHost[host] = machine
				m.doc("nodes", "machines", m.id("machine-generated", strconv.FormatInt(machine, 10)), p.Record{"name": host, "number": machine, "key": m.newKey("machine:" + host), "enabled": true, "createdAt": m.S.ExportedAt})
			}
		}
		settings := object(row, "protocol_settings")
		if len(settings) == 0 {
			for key, value := range row {
				settings[key] = value
			}
		}
		protocol = strings.ToLower(protocol)
		if protocol == "hysteria" && integer(settings, "version") == 2 {
			protocol = "hysteria2"
		}
		groups := []string{}
		for _, group := range groupIDs(first(row, "group_ids", "group_id")) {
			groups = append(groups, m.group(group))
		}
		port := first(row, "port", "server_port")
		portValue := integer(Row{"v": port}, "v")
		if portValue == 0 || strings.Contains(text(Row{"v": port}, "v"), "-") {
			m.fail("节点 %s 的端口无法无损映射：%v", text(row, "name"), port)
		}
		serverPort := integer(row, "server_port")
		if serverPort == 0 {
			serverPort = portValue
		}
		tlsMode := integer(settings, "tls")
		if protocol == "trojan" || protocol == "hysteria2" || protocol == "tuic" || protocol == "anytls" {
			if tlsMode == 0 {
				tlsMode = 1
			}
		}
		certificate := object(row, "cert_config")
		if len(certificate) == 0 {
			certificate = object(settings, "cert_config")
		}
		tlsSettings := object(settings, "tls_settings")
		if len(tlsSettings) == 0 {
			tlsSettings = object(settings, "tlsSettings")
		}
		if len(tlsSettings) == 0 {
			tlsSettings = object(settings, "tls")
		}
		if value := settings["allow_insecure"]; value != nil && tlsSettings["allow_insecure"] == nil {
			tlsSettings["allow_insecure"] = truth(settings, "allow_insecure")
		} else if tlsSettings["allow_insecure"] != nil {
			tlsSettings["allow_insecure"] = truth(tlsSettings, "allow_insecure")
		}
		reality := object(settings, "reality_settings")
		if len(reality) == 0 {
			reality = object(settings, "realitySettings")
		}
		if tlsMode == 2 {
			legacy := object(settings, "tls_settings")
			if len(legacy) == 0 {
				legacy = object(settings, "tlsSettings")
			}
			for key, value := range reality {
				legacy[key] = value
			}
			reality = legacy
			if reality["allow_insecure"] != nil {
				reality["allow_insecure"] = truth(reality, "allow_insecure")
			}
		}
		mode := "none"
		if tlsMode == 1 {
			mode = "tls"
		} else if tlsMode == 2 {
			mode = "reality"
		}
		transport := text(settings, "network")
		if protocol == "mieru" {
			transport = strings.ToLower(text(settings, "transport"))
			if value := object(settings, "transport"); len(value) > 0 {
				transport = strings.ToLower(text(value, "type"))
			}
			if transport == "" {
				transport = strings.ToLower(text(settings, "network"))
			}
			if transport != "udp" {
				transport = "tcp"
			}
			tlsMode, mode = 0, "none"
		}
		if transport == "" {
			transport = "tcp"
		}
		networkSettings := object(settings, "network_settings")
		if len(networkSettings) == 0 {
			networkSettings = object(settings, "networkSettings")
		}
		if !supportedProtocol(protocol) {
			m.fail("节点 %s 使用尚不支持的协议 %s，原始记录已保留", text(row, "name"), protocol)
		}
		if protocol == "tuic" && integer(settings, "version") == 4 {
			m.fail("节点 %s 使用 TUIC v4，不能当作 TUIC v5 无损接入", text(row, "name"))
		}
		if !cAllowedTransport(protocol, transport) {
			m.fail("节点 %s 的传输方式 %s 暂不能由 NodeRS 接入", text(row, "name"), transport)
		}
		for _, field := range []string{"routes", "route_ids", "custom_routes", "custom_outbounds", "fallbacks", "fallback", "fallbackForAlpn"} {
			if configured(settings[field]) || configured(row[field]) {
				m.fail("节点 %s 含尚不能转换的 %s 规则", text(row, "name"), field)
			}
		}
		if truth(row, "rate_time_enable") {
			m.fail("节点 %s 使用分时流量倍率，需要先确认倍率规则", text(row, "name"))
		}
		enabled := truth(row, "show")
		if _, ok := row["enabled"]; ok {
			enabled = truth(row, "enabled")
		}
		serverName := first(settings, "server_name", "serverName")
		if serverName == nil {
			serverName = first(tlsSettings, "server_name", "serverName")
		}
		if mode == "reality" {
			serverName = first(reality, "server_name", "serverName")
		}
		visible := truth(row, "show")
		if row["show"] == nil {
			visible = enabled
		}
		n := p.Record{"wireId": wireID, "machineId": machine, "name": row["name"], "host": host, "port": portValue, "serverPort": serverPort, "engine": "noders", "protocol": protocol, "tls": tlsMode > 0, "tlsMode": mode, "serverName": serverName, "tlsSettings": tlsSettings, "realitySettings": reality, "certConfig": certificate, "transport": transport, "transportSettings": networkSettings, "protocolOptions": settings, "groups": strings.Join(groups, ","), "trafficRate": number(row, "rate"), "enabled": enabled, "visible": visible, "source": m.ref(row, table), "createdAt": m.created(row, "created_at")}
		n["sort"] = integer(row, "sort")
		if number(row, "rate") == 0 {
			m.fail("节点 %s 的流量倍率为 0，请在源面板确认", text(row, "name"))
		}
		if integer(row, "parent_id") > 0 {
			n["parentSourceId"] = row["parent_id"]
			n["parentId"] = m.id("node", table+":"+text(row, "parent_id"))
			found := false
			for _, parent := range m.S.Rows(table) {
				if text(parent, "id") == text(row, "parent_id") {
					found = true
					break
				}
			}
			if !found {
				m.fail("节点 %s 的父节点不存在", text(row, "name"))
			}
		}
		m.doc("nodes", "nodes", m.id("node", table+":"+text(row, "id")), n)
	}
	if table, rows := m.table("v2_server"); rows != nil {
		for _, row := range rows {
			mapNode(row, table, text(row, "type"), integer(row, "id"))
		}
		return
	}
	for _, protocol := range []string{"shadowsocks", "vmess", "vless", "trojan", "hysteria", "tuic", "anytls", "sudoku", "mieru", "naive"} {
		table, rows := m.table("v2_server_" + protocol)
		for _, row := range rows {
			m.NodeCounter++
			mapNode(row, table, protocol, m.NodeCounter)
		}
	}
}
func first(row Row, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok && value != nil {
			if s, ok := value.(string); ok && s == "" {
				continue
			}
			return value
		}
	}
	return nil
}
func supportedProtocol(protocol string) bool {
	switch protocol {
	case "shadowsocks", "vmess", "vless", "trojan", "hysteria2", "tuic", "anytls", "mieru", "sudoku", "naive":
		return true
	}
	return false
}

func cAllowedTransport(protocol, transport string) bool {
	if protocol == "vless" || protocol == "vmess" || protocol == "trojan" {
		switch transport {
		case "", "tcp", "raw", "ws", "websocket", "httpupgrade", "grpc", "h2", "http", "xhttp", "splithttp":
			return true
		}
		return false
	}
	if protocol == "hysteria2" || protocol == "tuic" {
		return transport == "tcp" || transport == "udp" || transport == "quic" || transport == ""
	}
	if protocol == "mieru" && transport == "udp" {
		return true
	}
	return transport == "tcp" || transport == "raw" || transport == ""
}
func configured(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case Row:
		return len(v) > 0
	case string:
		return v != "" && v != "[]" && v != "null" && v != "0" && v != "false"
	case bool:
		return v
	case json.Number:
		return v != "0"
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return true
	}
}

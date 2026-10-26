package migration

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
		reality := object(settings, "reality_settings")
		mode := "none"
		if tlsMode == 1 {
			mode = "tls"
		} else if tlsMode == 2 {
			mode = "reality"
		}
		transport := text(settings, "network")
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
		if !cAllowedTransport(protocol, transport) {
			m.fail("节点 %s 的传输方式 %s 暂不能由 NodeRS 接入", text(row, "name"), transport)
		}
		for _, field := range []string{"routes", "route_ids", "custom_routes", "custom_outbounds", "fallbacks", "fallback", "fallbackForAlpn"} {
			if value := settings[field]; configured(value) {
				m.fail("节点 %s 含尚不能转换的 %s 规则", text(row, "name"), field)
			}
		}
		enabled := truth(row, "show")
		if _, ok := row["enabled"]; ok {
			enabled = truth(row, "enabled")
		}
		n := p.Record{"wireId": wireID, "machineId": machine, "name": row["name"], "host": host, "port": portValue, "serverPort": serverPort, "engine": "noders", "protocol": protocol, "tls": tlsMode > 0, "tlsMode": mode, "serverName": first(settings, "server_name", "serverName"), "tlsSettings": tlsSettings, "realitySettings": reality, "certConfig": certificate, "transport": transport, "transportSettings": networkSettings, "protocolOptions": settings, "groups": strings.Join(groups, ","), "trafficRate": number(row, "rate"), "enabled": enabled, "source": m.ref(row, table), "createdAt": m.created(row, "created_at")}
		if number(row, "rate") == 0 {
			m.fail("节点 %s 的流量倍率为 0，请在源面板确认", text(row, "name"))
		}
		if integer(row, "parent_id") > 0 {
			n["parentSourceId"] = row["parent_id"]
			m.warning("节点 %s 的父节点关系已保留，监听前请确认中转端口", text(row, "name"))
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
func (m *mapper) ssNodes() {
	m.warning("节点记录已转换；服务器证书、私钥文件和外部服务不在迁移快照中，请在接入 NodeRS 前核对证书与监听设置。")
	table, rows := m.table("node", "ss_node")
	machineByHost := map[string]int64{}
	machineCounter := int64(0)
	for _, row := range rows {
		host := text(row, "server")
		machine := machineByHost[host]
		if machine == 0 {
			machineCounter++
			machine = machineCounter
			machineByHost[host] = machine
			key := text(row, "password")
			if key == "" {
				key = m.newKey("machine:" + host)
			}
			m.doc("nodes", "machines", m.id("machine", strconv.FormatInt(machine, 10)), p.Record{"number": machine, "name": host, "key": key, "enabled": true, "source": m.ref(row, table), "createdAt": m.S.ExportedAt})
		}
		custom := object(row, "custom_config")
		sortValue := integer(row, "sort")
		protocol := map[int64]string{0: "shadowsocks", 1: "shadowsocks", 2: "tuic", 11: "vmess", 14: "trojan"}[sortValue]
		if protocol == "" {
			m.fail("SSPanel 节点 %s 的接入类型 %d 尚未支持，原始记录已保留", text(row, "name"), sortValue)
			continue
		}
		if truth(row, "is_dynamic_rate") {
			m.fail("节点 %s 使用动态倍率，需要先实现或确认倍率规则，不能静默改成固定倍率", text(row, "name"))
		}
		port := integer(custom, "offset_port_user")
		if port == 0 {
			port = integer(custom, "offset_port_node")
		}
		if port == 0 {
			port = 443
		}
		serverPort := integer(custom, "offset_port_node")
		if serverPort == 0 {
			serverPort = port
		}
		transport := text(custom, "network")
		if transport == "" {
			transport = "tcp"
		}
		base := p.Record{"name": row["name"], "host": host, "port": port, "serverPort": serverPort, "machineId": machine, "engine": "noders", "protocol": protocol, "tls": sortValue != 0 && sortValue != 1, "tlsMode": "tls", "serverName": custom["host"], "transport": transport, "protocolOptions": custom, "transportSettings": p.Record{"path": custom["path"], "serviceName": custom["servicename"]}, "certConfig": custom["cert_config"], "groups": m.group(text(row, "node_group")), "minimumAccessLevel": integer(row, "node_class"), "trafficRate": number(row, "traffic_rate"), "enabled": truth(row, "type"), "source": m.ref(row, table), "createdAt": m.S.ExportedAt}
		if !cAllowedTransport(protocol, transport) {
			m.fail("节点 %s 的传输方式 %s 暂不能由 NodeRS 接入", text(row, "name"), transport)
		}
		if sortValue == 0 {
			for _, user := range m.S.Rows("user") {
				m.NodeCounter++
				copy := p.Record{}
				for k, v := range base {
					copy[k] = v
				}
				copy["wireId"] = m.NodeCounter
				copy["name"] = fmt.Sprintf("%s · %s", text(row, "name"), text(user, "email"))
				copy["port"] = integer(user, "port")
				copy["serverPort"] = integer(user, "port")
				copy["allowedAccountIds"] = []int64{integer(user, "id")}
				copy["protocolOptions"] = p.Record{"cipher": user["method"]}
				copy["tlsMode"] = "none"
				copy["passwordMode"] = "proxy"
				m.doc("nodes", "nodes", m.id("node", text(row, "id")+":user:"+text(user, "id")), copy)
			}
			continue
		}
		if sortValue == 1 {
			method := text(custom, "method")
			if method == "" {
				method = "2022-blake3-aes-128-gcm"
			}
			custom["cipher"] = method
			custom["serverKey"] = custom["server_key"]
			base["passwordMode"] = "sha3-hex-prefix-base64"
			if !strings.Contains(method, "aes-128") && !strings.Contains(method, "aes-256") {
				m.fail("SS2022 节点 %s 的算法 %s 不适用于 NodeRS 多用户模式", text(row, "name"), method)
			}
		} else if protocol == "trojan" {
			base["passwordMode"] = "uuid"
		} else {
			base["passwordMode"] = "proxy"
		}
		m.NodeCounter++
		base["wireId"] = m.NodeCounter
		m.doc("nodes", "nodes", m.id("node", text(row, "id")), base)
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
		return v != "" && v != "[]" && v != "null" && v != "0"
	default:
		return true
	}
}

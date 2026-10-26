package migration

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strconv"
	"strings"
)

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
		if sortValue == 14 {
			if kind := text(object(custom, "header"), "type"); kind != "" && kind != "none" {
				transport = kind
			}
		}
		if transport == "" {
			transport = "tcp"
		}
		tls := sortValue == 2 || sortValue == 14 || sortValue == 11 && text(custom, "security") == "tls"
		tlsMode := "none"
		if tls {
			tlsMode = "tls"
		}
		group := text(row, "node_group")
		if group == "" {
			group = "0"
		}
		settings := ssTransport(custom, transport)
		serverName := text(custom, "host")
		if serverName == "" {
			headers := object(object(object(custom, "header"), "request"), "headers")
			if hosts, ok := headers["Host"].([]any); ok && len(hosts) > 0 {
				serverName = text(Row{"host": hosts[0]}, "host")
			}
		}
		if sortValue == 11 {
			custom["cipher"] = first(custom, "encryption", "cipher")
			delete(custom, "security")
		}
		base := p.Record{"name": row["name"], "host": host, "port": port, "serverPort": serverPort, "machineId": machine, "engine": "noders", "protocol": protocol, "tls": tls, "tlsMode": tlsMode, "serverName": serverName, "tlsSettings": p.Record{"allow_insecure": truth(custom, "allow_insecure")}, "transport": transport, "protocolOptions": custom, "transportSettings": settings, "certConfig": custom["cert_config"], "groups": m.group(group), "minimumAccessLevel": integer(row, "node_class"), "trafficRate": number(row, "traffic_rate"), "enabled": truth(row, "type"), "source": m.ref(row, table), "createdAt": m.S.ExportedAt}
		if !cAllowedTransport(protocol, transport) {
			m.fail("节点 %s 的传输方式 %s 暂不能由 NodeRS 接入", text(row, "name"), transport)
		}
		if sortValue == 0 {
			for _, user := range m.S.Rows("user") {
				if integer(user, "port") <= 0 || text(user, "method") == "" {
					m.fail("用户 %s 缺少传统 Shadowsocks 端口或算法", text(user, "id"))
				}
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

func ssTransport(custom Row, transport string) p.Record {
	key := map[string]string{"ws": "ws", "websocket": "ws", "httpupgrade": "ws", "grpc": "grpc", "h2": "h2", "http": "http"}[transport]
	settings := object(custom, key+"-opts")
	if len(settings) == 0 {
		settings = object(custom, key+"_opts")
	}
	if len(settings) == 0 {
		settings = Row{"path": custom["path"], "headers": custom["headers"]}
	}
	if service := first(settings, "grpc-service-name", "serviceName"); service != nil {
		settings["serviceName"] = service
	} else {
		settings["serviceName"] = first(custom, "servicename", "serviceName")
	}
	return p.Record(settings)
}

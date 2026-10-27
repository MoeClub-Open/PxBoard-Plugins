package nodes

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var tlsProtocols = []string{"vless", "vmess", "trojan", "hysteria2", "anytls", "tuic", "naive"}

func resource() p.Resource {
	fields := []p.Field{
		c.Field("name", "节点名称", "text", true),
		c.Field("region", "地区", "text", false),
		c.Select("engine", "代理内核", "noders|NodeRS", "xray|Xray", "sing-box|sing-box"),
		c.Select("protocol", "协议", "anytls|AnyTLS", "mieru|Mieru", "vless|VLESS", "vmess|VMess", "trojan|Trojan", "shadowsocks|Shadowsocks", "hysteria2|Hysteria 2", "tuic|TUIC", "sudoku|Sudoku", "naive|Naive"),
		c.Field("host", "连接地址", "text", true),
		c.Field("port", "连接端口", "number", true),
		{Key: "serverPort", Label: "节点监听端口", Type: "number", Hint: "留空使用连接端口", When: map[string][]string{"engine": {"noders"}}},
		{Key: "transport", Label: "传输方式", Type: "select", Required: true, Default: "tcp", Options: []p.Option{{Label: "TCP", Value: "tcp"}, {Label: "UDP", Value: "udp"}}, When: map[string][]string{"engine": {"noders"}, "protocol": {"mieru"}}},
		{Key: "tls", Label: "启用 TLS", Type: "boolean", Default: true, When: map[string][]string{"protocol": {"vless", "vmess"}}},
		{Key: "serverName", Label: "TLS 域名", Type: "text", When: map[string][]string{"protocol": tlsProtocols}},
		{Key: "certPath", Label: "节点证书路径", Type: "text", When: map[string][]string{"engine": {"xray", "sing-box"}, "protocol": tlsProtocols}},
		{Key: "keyPath", Label: "节点私钥路径", Type: "text", When: map[string][]string{"engine": {"xray", "sing-box"}, "protocol": tlsProtocols}},
	}
	fields = append(fields, certificateFields()...)
	fields = append(fields,
		c.GroupField("groups", "节点分组", true),
		c.Field("trafficRate", "流量倍率", "number", true),
		p.Field{Key: "visible", Label: "在订阅中显示", Type: "boolean", Default: true},
		p.Field{Key: "enabled", Label: "启用独立服务", Type: "boolean", Hint: "关闭后不在机器上单独运行；复用主节点的入口仍由主节点提供服务"},
	)
	return p.Resource{
		Key: "nodes", Title: "节点", Endpoint: "/nodes", CreateLabel: "添加节点", Editable: true, Deletable: true,
		Columns: []p.Field{c.Field("name", "节点名称", "text", false), c.Field("region", "地区", "text", false), c.Field("host", "服务器", "text", false), c.GroupField("groups", "节点分组", false), c.Field("protocol", "协议", "tag", false), c.Field("lastSeen", "最近在线", "datetime", false), c.Field("error", "运行错误", "text", false), c.Field("visible", "订阅显示", "boolean", false), c.Field("enabled", "运行方式", "nodeService", false)},
		Fields:  fields, Actions: []p.Action{{Key: "rotate", Label: "接入令牌", Confirm: "生成新的节点接入令牌？原令牌会立即失效。"}},
	}
}

func certificateFields() []p.Field {
	fields := []p.Field{
		{Key: "certConfig.mode", Label: "证书方式", Type: "select", Default: "http", Options: []p.Option{{Label: "HTTP 验证签发", Value: "http"}, {Label: "DNS 验证签发", Value: "dns"}, {Label: "节点本地文件", Value: "file"}}},
		{Key: "certConfig.cert_path", Label: "节点证书路径", Type: "text", When: map[string][]string{"certConfig.mode": {"file", "path"}}},
		{Key: "certConfig.key_path", Label: "节点私钥路径", Type: "text", When: map[string][]string{"certConfig.mode": {"file", "path"}}},
		{Key: "certConfig.domain", Label: "签发域名", Type: "text", Hint: "留空使用 TLS 域名或连接地址", When: map[string][]string{"certConfig.mode": {"http", "acme", "letsencrypt", "dns"}}},
		{Key: "certConfig.email", Label: "证书联系邮箱", Type: "email", When: map[string][]string{"certConfig.mode": {"http", "acme", "letsencrypt", "dns"}}},
		{Key: "certConfig.provider", Label: "DNS 服务商", Type: "select", Options: []p.Option{{Label: "Cloudflare", Value: "cloudflare"}, {Label: "阿里云 DNS", Value: "alidns"}}, When: map[string][]string{"certConfig.mode": {"dns"}}},
		{Key: "certConfig.api_token", Label: "Cloudflare API 令牌", Type: "password", When: map[string][]string{"certConfig.mode": {"dns"}, "certConfig.provider": {"cloudflare"}}},
		{Key: "certConfig.access_key_id", Label: "阿里云 AccessKey ID", Type: "text", When: map[string][]string{"certConfig.mode": {"dns"}, "certConfig.provider": {"alidns"}}},
		{Key: "certConfig.access_key_secret", Label: "阿里云 AccessKey Secret", Type: "password", When: map[string][]string{"certConfig.mode": {"dns"}, "certConfig.provider": {"alidns"}}},
	}
	for i := range fields {
		if fields[i].When == nil {
			fields[i].When = map[string][]string{}
		}
		fields[i].When["engine"] = []string{"noders"}
		fields[i].When["protocol"] = tlsProtocols
	}
	return fields
}

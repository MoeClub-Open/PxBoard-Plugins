package nodes

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var tlsProtocols = []string{"vless", "vmess", "trojan", "hysteria2", "anytls", "tuic", "naive", "flux"}

func resource() p.Resource {
	fields := []p.Field{
		c.Field("name", "节点名称", "text", true),
		c.Select("engine", "代理内核", "noders|NodeRS", "xray|Xray", "sing-box|sing-box"),
		c.Select("protocol", "协议", "anytls|AnyTLS", "flux|Flux", "mieru|Mieru", "vless|VLESS", "vmess|VMess", "trojan|Trojan", "shadowsocks|Shadowsocks", "hysteria2|Hysteria 2", "tuic|TUIC", "sudoku|Sudoku", "naive|Naive"),
		c.Field("host", "连接地址", "text", true),
		c.Field("port", "连接端口", "number", true),
		{Key: "serverPort", Label: "节点监听端口", Type: "number", Hint: "留空使用连接端口", When: map[string][]string{"engine": {"noders"}}},
		{Key: "transport", Label: "传输方式", Type: "select", Required: true, Default: "tcp", Options: []p.Option{{Label: "TCP", Value: "tcp"}, {Label: "UDP", Value: "udp"}}, When: map[string][]string{"engine": {"noders"}, "protocol": {"mieru"}}},
		{Key: "tls", Label: "启用 TLS", Type: "boolean", Default: true, When: map[string][]string{"protocol": {"vless", "vmess"}}},
		{Key: "serverName", Label: "TLS 域名", Type: "text", When: map[string][]string{"protocol": tlsProtocols}},
		{Key: "certPath", Label: "节点证书路径", Type: "text", When: map[string][]string{"engine": {"xray", "sing-box"}, "protocol": tlsProtocols}},
		{Key: "keyPath", Label: "节点私钥路径", Type: "text", When: map[string][]string{"engine": {"xray", "sing-box"}, "protocol": tlsProtocols}},
	}
	fields = append(fields, fluxFields()...)
	fields = append(fields, certificateFields()...)
	fields = append(fields,
		c.GroupField("groups", "节点分组", true),
		c.Field("trafficRate", "流量倍率", "number", true),
		p.Field{Key: "visible", Label: "在订阅中显示", Type: "boolean", Default: true},
		p.Field{Key: "enabled", Label: "启用独立服务", Type: "boolean", Hint: "关闭后不在服务器上单独运行；复用主节点的入口仍由主节点提供服务"},
	)
	return p.Resource{Renderer: "node-connections",
		Key: "nodes", Title: "节点", Endpoint: "/nodes", CreateLabel: "添加节点", Editable: true, Deletable: true,
		Columns: []p.Field{
			c.Field("name", "节点名称", "text", false),
			c.Field("host", "连接地址", "text", false),
			c.Field("port", "端口", "number", false),
			c.Field("protocol", "协议", "tag", false),
			c.Field("machineId", "所属服务器", "select", false),
			c.GroupField("groups", "节点分组", false),
			c.Field("trafficRate", "倍率", "number", false),
			c.Field("visible", "订阅显示", "boolean", false),
			c.Field("enabled", "运行方式", "nodeService", false),
		},
		Fields:  fields, Actions: []p.Action{{Key: "rotate", Label: "接入令牌", Confirm: "生成新的节点接入令牌？原令牌会立即失效。"}},
	}
}

func fluxFields() []p.Field {
	fluxOnly := map[string][]string{"engine": {"noders"}, "protocol": {"flux"}}
	return []p.Field{
		{
			Key: "protocolOptions.security", Label: "默认安全方式", Type: "select", Default: "reality",
			Options: []p.Option{
				{Label: "REALITY 证书借用", Value: "reality"},
				{Label: "TLS 证书", Value: "tls"},
				{Label: "数据包混淆", Value: "obfs"},
			},
			When: fluxOnly,
		},
		{
			Key: "protocolOptions.ipStrategy", Label: "双栈调度策略", Type: "select", Default: "dual",
			Options: []p.Option{
				{Label: "双栈并行", Value: "dual"},
				{Label: "IPv4 优先", Value: "prefer_v4"},
				{Label: "IPv6 优先", Value: "prefer_v6"},
				{Label: "仅 IPv4", Value: "v4_only"},
				{Label: "仅 IPv6", Value: "v6_only"},
			},
			When: fluxOnly,
		},
		{Key: "protocolOptions.standbyChannels", Label: "热备通道数", Type: "number", Default: 2, Hint: "保持预热的备用通道数量（0-16）", When: fluxOnly},
		{Key: "protocolOptions.rotationIntervalSec", Label: "主动轮换周期（秒）", Type: "number", Default: 180, Hint: "0 表示关闭定时轮换", When: fluxOnly},
		{Key: "protocolOptions.rotationBytesMb", Label: "主动轮换流量（MiB）", Type: "number", Default: 96, Hint: "0 表示关闭按流量轮换", When: fluxOnly},
		{
			Key: "protocolOptions.reality.serverName", Label: "借用域名（SNI）", Type: "text", Default: "www.microsoft.com",
			Hint: "目标站点域名，留空使用 TLS 域名或 www.microsoft.com",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality"}},
		},
		{
			Key: "protocolOptions.reality.dest", Label: "回源目标地址", Type: "text",
			Hint: "例如 www.microsoft.com:443，留空自动使用借用域名:443",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality"}},
		},
		{
			Key: "protocolOptions.reality.privateKey", Label: "REALITY 私钥", Type: "password",
			Hint: "留空保存时自动生成 X25519 密钥对",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality"}},
		},
		{
			Key: "protocolOptions.reality.publicKey", Label: "REALITY 公钥", Type: "text",
			Hint: "留空自动从私钥推导",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality"}},
		},
		{
			Key: "protocolOptions.reality.shortId", Label: "REALITY Short ID", Type: "text",
			Hint: "偶数位十六进制（最长 16 位），留空自动生成",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality"}},
		},
		{
			Key: "protocolOptions.clientFingerprint", Label: "客户端指纹", Type: "select", Default: "chrome",
			Options: []p.Option{
				{Label: "Chrome", Value: "chrome"},
				{Label: "Firefox", Value: "firefox"},
				{Label: "Safari", Value: "safari"},
				{Label: "iOS", Value: "ios"},
				{Label: "Edge", Value: "edge"},
				{Label: "随机", Value: "random"},
			},
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"reality", "tls"}},
		},
		{
			Key: "tlsSettings.allow_insecure", Label: "跳过客户端证书校验", Type: "boolean", Default: false,
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"tls"}},
		},
		{
			Key: "protocolOptions.obfsKey", Label: "默认混淆密钥", Type: "text",
			Hint: "留空保存时自动生成 32 位随机密钥",
			When: map[string][]string{"engine": {"noders"}, "protocol": {"flux"}, "protocolOptions.security": {"obfs"}},
		},
		{
			Key: "protocolOptions.channels", Label: "多通道配置", Type: "fluxChannels",
			Hint: "留空默认按节点连接地址与端口自动启用 TCP + UDP 双通道",
			When: fluxOnly,
		},
	}
}

func certificateFields() []p.Field {
	fields := []p.Field{
		{Key: "certConfig.mode", Label: "证书方式", Type: "select", Default: "http", Hint: "TLS 模式或复用节点证书的通道使用", Options: []p.Option{{Label: "HTTP 验证签发", Value: "http"}, {Label: "DNS 验证签发", Value: "dns"}, {Label: "节点本地文件", Value: "file"}}},
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

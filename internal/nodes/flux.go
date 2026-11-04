package nodes

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const maxFluxExpandedPorts = 64

func normalizeChannelSlice(raw any) []any {
	switch items := raw.(type) {
	case nil:
		return []any{}
	case []any:
		out := make([]any, 0, len(items))
		for _, item := range items {
			switch m := item.(type) {
			case map[string]any:
				out = append(out, cloneMap(m))
			case p.Record:
				out = append(out, cloneMap(map[string]any(m)))
			}
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(items))
		for _, m := range items {
			out = append(out, cloneMap(m))
		}
		return out
	case []p.Record:
		out := make([]any, 0, len(items))
		for _, m := range items {
			out = append(out, cloneMap(map[string]any(m)))
		}
		return out
	default:
		return []any{}
	}
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = cloneAny(v)
	}
	return out
}

func cloneAny(val any) any {
	switch typed := val.(type) {
	case map[string]any:
		return cloneMap(typed)
	case p.Record:
		return cloneMap(map[string]any(typed))
	case []any:
		cp := make([]any, len(typed))
		for i, item := range typed {
			cp[i] = cloneAny(item)
		}
		return cp
	case []string:
		cp := make([]string, len(typed))
		copy(cp, typed)
		return cp
	case map[string]string:
		cp := make(map[string]string, len(typed))
		for k, v := range typed {
			cp[k] = v
		}
		return cp
	default:
		return val
	}
}

func portSpecString(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(int(v))
	case int:
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(v)
	case int64:
		if v <= 0 {
			return ""
		}
		return strconv.FormatInt(v, 10)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s := portSpecString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func parseFluxPortSpec(spec string, maxPorts int) ([]int, error) {
	tokens := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
	if len(tokens) == 0 {
		return nil, fmt.Errorf("Flux 端口格式不能为空")
	}
	seen := make(map[int]bool)
	ports := make([]int, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if startStr, endStr, isRange := strings.Cut(token, "-"); isRange {
			start, err1 := strconv.Atoi(strings.TrimSpace(startStr))
			end, err2 := strconv.Atoi(strings.TrimSpace(endStr))
			if err1 != nil || err2 != nil || start < 1 || end > 65535 || start > end {
				return nil, fmt.Errorf("Flux 端口段无效：%s", token)
			}
			if len(ports)+(end-start+1) > maxPorts {
				return nil, fmt.Errorf("Flux 展开端口总数不能超过 %d", maxPorts)
			}
			for port := start; port <= end; port++ {
				if !seen[port] {
					seen[port] = true
					ports = append(ports, port)
				}
			}
		} else {
			port, err := strconv.Atoi(token)
			if err != nil || port < 1 || port > 65535 {
				return nil, fmt.Errorf("Flux 端口无效：%s", token)
			}
			if !seen[port] {
				if len(ports) >= maxPorts {
					return nil, fmt.Errorf("Flux 展开端口总数不能超过 %d", maxPorts)
				}
				seen[port] = true
				ports = append(ports, port)
			}
		}
	}
	if len(ports) == 0 {
		return nil, fmt.Errorf("Flux 端口格式不能为空")
	}
	return ports, nil
}

func decodeRealityKey(encoded string) ([]byte, error) {
	trimmed := strings.TrimSpace(encoded)
	for _, enc := range []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	} {
		if decoded, err := enc.DecodeString(trimmed); err == nil {
			if len(decoded) != 32 {
				return nil, fmt.Errorf("REALITY 密钥长度必须为 32 字节")
			}
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("REALITY 密钥 Base64 格式无效")
}

func deriveRealityPublicKey(privEncoded string) (string, error) {
	raw, err := decodeRealityKey(privEncoded)
	if err != nil {
		return "", err
	}
	priv, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("REALITY 私钥无效：%w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}

func generateRealityKeypair() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64
	priv, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}

func validateFluxShortID(sid string) error {
	trimmed := strings.TrimSpace(sid)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) > 16 || len(trimmed)%2 != 0 {
		return fmt.Errorf("REALITY Short ID 需为不超过 16 位的偶数长度十六进制字符串")
	}
	if _, err := hex.DecodeString(trimmed); err != nil {
		return fmt.Errorf("REALITY Short ID 仅支持十六进制字符")
	}
	return nil
}

func generateFluxShortID() (string, error) {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func generateFluxObfsKey() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf)[:32], nil
}

func normalizeIpStrategy(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "dual", "auto", "both", "dual-stack":
		return "dual", nil
	case "prefer_v4", "prefer-v4", "ipv4_first":
		return "prefer_v4", nil
	case "prefer_v6", "prefer-v6", "ipv6_first":
		return "prefer_v6", nil
	case "v4_only", "v4-only", "ipv4", "ipv4_only":
		return "v4_only", nil
	case "v6_only", "v6-only", "ipv6", "ipv6_only":
		return "v6_only", nil
	default:
		return "", fmt.Errorf("Flux 双栈调度策略无效：%s", raw)
	}
}

func normalizeFluxSecurity(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "reality":
		return "reality", nil
	case "tls":
		return "tls", nil
	case "obfs", "salamander":
		return "obfs", nil
	default:
		return "", fmt.Errorf("Flux 安全方式无效：%s", raw)
	}
}

func normalizeFluxTransport(rawTransport, rawHttpMode string) (string, string, error) {
	t := strings.ToLower(strings.TrimSpace(rawTransport))
	m := strings.ToLower(strings.TrimSpace(rawHttpMode))
	switch t {
	case "", "tcp", "raw":
		return "tcp", "", nil
	case "udp", "quic", "h3":
		return "udp", "", nil
	case "ws", "websocket":
		return "http", "ws", nil
	case "httpupgrade", "http-upgrade", "http_upgrade", "upgrade":
		return "http", "httpupgrade", nil
	case "h2", "http2":
		return "http", "h2", nil
	case "http":
		switch m {
		case "", "ws", "websocket":
			return "http", "ws", nil
		case "httpupgrade", "http-upgrade", "http_upgrade", "upgrade":
			return "http", "httpupgrade", nil
		case "h2", "http2", "http":
			return "http", "h2", nil
		default:
			return "", "", fmt.Errorf("Flux HTTP 封装模式无效：%s", rawHttpMode)
		}
	default:
		return "", "", fmt.Errorf("Flux 通道传输方式无效：%s", rawTransport)
	}
}

func normalizeHeadersMap(raw any) map[string]string {
	out := map[string]string{}
	switch m := raw.(type) {
	case map[string]string:
		for k, v := range m {
			if kt := strings.TrimSpace(k); kt != "" {
				out[kt] = strings.TrimSpace(v)
			}
		}
	case map[string]any:
		for k, v := range m {
			if kt := strings.TrimSpace(k); kt != "" && v != nil {
				out[kt] = strings.TrimSpace(fmt.Sprint(v))
			}
		}
	case p.Record:
		for k, v := range m {
			if kt := strings.TrimSpace(k); kt != "" && v != nil {
				out[kt] = strings.TrimSpace(fmt.Sprint(v))
			}
		}
	}
	return out
}

func headersSignature(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+headers[k])
	}
	return strings.Join(parts, "&")
}

func validateRealityDest(dest string) error {
	trimmed := strings.TrimSpace(dest)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "[") {
		host, portStr, err := net.SplitHostPort(trimmed)
		if err != nil || strings.TrimSpace(host) == "" {
			return fmt.Errorf("REALITY 回源目标地址无效：%s", dest)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("REALITY 回源目标端口无效：%s", dest)
		}
		return nil
	}
	if strings.Count(trimmed, ":") == 1 {
		host, portStr, err := net.SplitHostPort(trimmed)
		if err != nil || strings.TrimSpace(host) == "" {
			return fmt.Errorf("REALITY 回源目标地址无效：%s", dest)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("REALITY 回源目标端口无效：%s", dest)
		}
	}
	return nil
}

func intOption(m map[string]any, defaultVal int, keys ...string) (int, error) {
	for _, k := range keys {
		raw, exists := m[k]
		if !exists || raw == nil || raw == "" {
			continue
		}
		switch v := raw.(type) {
		case float64:
			return int(v), nil
		case int:
			return v, nil
		case int64:
			return int(v), nil
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return 0, err
			}
			return n, nil
		}
	}
	return defaultVal, nil
}

func boolTrueDefault(m map[string]any, key string) bool {
	raw, exists := m[key]
	if !exists || raw == nil {
		return true
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s != "false" && s != "0" && s != "no" && s != "off"
	case float64:
		return v != 0
	case int:
		return v != 0
	default:
		return true
	}
}

type fluxBindKey struct {
	ip    string
	port  int
	isUDP bool
}

func normalizeFluxNode(b p.Record, creating, certificateChanged bool) error {
	if p.Text(b, "engine") != "noders" {
		return fmt.Errorf("Flux 协议请选择 NodeRS")
	}
	options, _ := b["protocolOptions"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	} else {
		options = cloneMap(options)
	}

	ipStrategy, err := normalizeIpStrategy(firstText(options, "ipStrategy", "ip_strategy"))
	if err != nil {
		return err
	}
	options["ipStrategy"] = ipStrategy
	delete(options, "ip_strategy")

	standby, err := intOption(options, 2, "standbyChannels", "standby_channels")
	if err != nil || standby < 0 || standby > 16 {
		return fmt.Errorf("Flux 热备通道数需在 0 到 16 之间")
	}
	options["standbyChannels"] = standby
	delete(options, "standby_channels")

	rotInterval, err := intOption(options, 180, "rotationIntervalSec", "rotation_interval_sec")
	if err != nil || rotInterval < 0 {
		return fmt.Errorf("Flux 主动轮换周期无效")
	}
	options["rotationIntervalSec"] = rotInterval
	delete(options, "rotation_interval_sec")

	rotBytes, err := intOption(options, 96, "rotationBytesMb", "rotation_bytes_mb")
	if err != nil || rotBytes < 0 {
		return fmt.Errorf("Flux 主动轮换流量阈值无效")
	}
	options["rotationBytesMb"] = rotBytes
	delete(options, "rotation_bytes_mb")

	rawSec := firstText(options, "security", "defaultSecurity", "default_security")
	if rawSec == "" {
		if p.Text(b, "tlsMode") == "tls" {
			rawSec = "tls"
		} else {
			rawSec = "reality"
		}
	}
	defaultSec, err := normalizeFluxSecurity(rawSec)
	if err != nil {
		return err
	}
	options["security"] = defaultSec
	delete(options, "defaultSecurity")
	delete(options, "default_security")

	fp := strings.ToLower(firstText(options, "clientFingerprint", "client_fingerprint"))
	delete(options, "client_fingerprint")
	if fp == "" {
		delete(options, "clientFingerprint")
	} else {
		if !c.Allowed(fp, "chrome", "firefox", "safari", "ios", "edge", "random", "android", "360", "qq") {
			return fmt.Errorf("客户端指纹不支持：%s", fp)
		}
		options["clientFingerprint"] = fp
	}

	realityMap, _ := options["reality"].(map[string]any)
	if realityMap == nil {
		if rs, ok := b["realitySettings"].(map[string]any); ok && len(rs) > 0 {
			realityMap = cloneMap(rs)
		} else {
			realityMap = map[string]any{}
		}
	} else {
		realityMap = cloneMap(realityMap)
	}

	nodePriv := firstText(realityMap, "privateKey", "private_key")
	nodePub := firstText(realityMap, "publicKey", "public_key")
	nodeShortID := firstText(realityMap, "shortId", "short_id")
	nodeRealitySNI := firstText(realityMap, "serverName", "server_name")
	nodeRealityDest := firstText(realityMap, "dest")

	if err := validateFluxShortID(nodeShortID); err != nil {
		return err
	}
	if err := validateRealityDest(nodeRealityDest); err != nil {
		return err
	}
	if nodePriv != "" {
		derivedPub, err := deriveRealityPublicKey(nodePriv)
		if err != nil {
			return err
		}
		nodePub = derivedPub
	} else if defaultSec == "reality" {
		genPriv, genPub, err := generateRealityKeypair()
		if err != nil {
			return fmt.Errorf("生成 REALITY 密钥失败：%w", err)
		}
		nodePriv, nodePub = genPriv, genPub
	}
	if defaultSec == "reality" {
		if nodeShortID == "" {
			sid, err := generateFluxShortID()
			if err != nil {
				return fmt.Errorf("生成 REALITY Short ID 失败：%w", err)
			}
			nodeShortID = sid
		}
		if nodeRealitySNI == "" {
			nodeRealitySNI = firstText(b, "serverName")
			if nodeRealitySNI == "" {
				nodeRealitySNI = "www.microsoft.com"
			}
		}
	}

	cleanNodeReality := map[string]any{}
	if nodeRealitySNI != "" {
		cleanNodeReality["serverName"] = nodeRealitySNI
	}
	if nodeRealityDest != "" {
		cleanNodeReality["dest"] = nodeRealityDest
	}
	if nodePriv != "" {
		cleanNodeReality["privateKey"] = nodePriv
	}
	if nodePub != "" {
		cleanNodeReality["publicKey"] = nodePub
	}
	if nodeShortID != "" {
		cleanNodeReality["shortId"] = nodeShortID
	}
	if len(cleanNodeReality) > 0 {
		options["reality"] = cleanNodeReality
		rs := p.Record{
			"server_name": nodeRealitySNI,
			"private_key": nodePriv,
			"public_key":  nodePub,
			"short_id":    nodeShortID,
		}
		if nodeRealityDest != "" {
			rs["dest"] = nodeRealityDest
		}
		b["realitySettings"] = rs
	} else {
		delete(options, "reality")
	}

	nodeObfsKey := firstText(options, "obfsKey", "obfs_key")
	delete(options, "obfs_key")
	if defaultSec == "obfs" && nodeObfsKey == "" {
		generated, err := generateFluxObfsKey()
		if err != nil {
			return fmt.Errorf("生成混淆密钥失败：%w", err)
		}
		nodeObfsKey = generated
	}
	if nodeObfsKey != "" {
		if len(nodeObfsKey) < 4 {
			return fmt.Errorf("Flux 混淆密钥长度至少为 4 位")
		}
		options["obfsKey"] = nodeObfsKey
	} else {
		delete(options, "obfsKey")
	}

	defaultNodePort := int(p.Number(b, "serverPort"))
	if defaultNodePort <= 0 {
		defaultNodePort = int(p.Number(b, "port"))
	}
	needsInheritedCert := defaultSec == "tls"

	rawChannels := normalizeChannelSlice(options["channels"])
	if len(rawChannels) == 0 {
		if defaultNodePort <= 0 || defaultNodePort > 65535 {
			return fmt.Errorf("Flux 节点监听端口无效")
		}
		delete(options, "channels")
	} else {
		normalizedChannels := make([]any, 0, len(rawChannels))
		enabledCount := 0
		seenBindings := map[fluxBindKey]string{}
		distinctServerPorts := 0

		for idx, item := range rawChannels {
			ch, ok := item.(map[string]any)
			if !ok {
				continue
			}
			outCh := map[string]any{}
			id := strings.TrimSpace(firstText(ch, "id"))
			if id == "" {
				id = fmt.Sprintf("ch-%d", idx+1)
			}
			outCh["id"] = id

			enabled := boolTrueDefault(ch, "enabled")
			outCh["enabled"] = enabled
			if enabled {
				enabledCount++
			}

			role := strings.ToLower(firstText(ch, "role"))
			switch role {
			case "", "primary":
				outCh["role"] = "primary"
			case "backup", "standby", "failover":
				outCh["role"] = "backup"
			default:
				return fmt.Errorf("通道 %s 的调度角色无效：%s", id, role)
			}

			weight, err := intOption(ch, 100, "weight")
			if err != nil || weight < 1 || weight > 100 {
				return fmt.Errorf("通道 %s 的权重需在 1 到 100 之间", id)
			}
			outCh["weight"] = weight

			if connectHost := firstText(ch, "connectHost", "connect_host", "server", "host"); connectHost != "" {
				outCh["connectHost"] = connectHost
			}

			connectPortsSpec := portSpecString(ch["connectPorts"])
			if connectPortsSpec == "" {
				connectPortsSpec = portSpecString(ch["connect_ports"])
			}
			if connectPortsSpec == "" {
				connectPortsSpec = portSpecString(ch["ports"])
			}
			if connectPortsSpec != "" && connectPortsSpec != "0" {
				if _, err := parseFluxPortSpec(connectPortsSpec, maxFluxExpandedPorts); err != nil {
					return fmt.Errorf("通道 %s 连接端口错误：%w", id, err)
				}
				outCh["connectPorts"] = connectPortsSpec
			}

			ipVer := strings.ToLower(firstText(ch, "ipVersion", "ip_version"))
			switch ipVer {
			case "", "auto", "dual", "both":
				outCh["ipVersion"] = "auto"
			case "ipv4", "v4", "4":
				outCh["ipVersion"] = "ipv4"
			case "ipv6", "v6", "6":
				outCh["ipVersion"] = "ipv6"
			default:
				return fmt.Errorf("通道 %s 的地址栈偏好无效：%s", id, ipVer)
			}

			listenIP := "0.0.0.0"
			if rawListenIP := firstText(ch, "listenIp", "listen_ip"); rawListenIP != "" {
				trimmedIP := strings.Trim(rawListenIP, "[]")
				parsedIP := net.ParseIP(trimmedIP)
				if parsedIP == nil {
					return fmt.Errorf("通道 %s 的本地监听地址无效：%s", id, rawListenIP)
				}
				listenIP = parsedIP.String()
				outCh["listenIp"] = listenIP
			}

			listenPortsSpec := portSpecString(ch["listenPorts"])
			if listenPortsSpec == "" {
				listenPortsSpec = portSpecString(ch["listen_ports"])
			}
			if listenPortsSpec != "" && listenPortsSpec != "0" {
				if _, err := parseFluxPortSpec(listenPortsSpec, maxFluxExpandedPorts); err != nil {
					return fmt.Errorf("通道 %s 本地监听端口错误：%w", id, err)
				}
				outCh["listenPorts"] = listenPortsSpec
			}

			effectiveListenSpec := listenPortsSpec
			if effectiveListenSpec == "" || effectiveListenSpec == "0" {
				effectiveListenSpec = connectPortsSpec
			}
			var serverPorts []int
			if effectiveListenSpec == "" || effectiveListenSpec == "0" {
				if defaultNodePort <= 0 || defaultNodePort > 65535 {
					return fmt.Errorf("通道 %s 未填写端口且节点默认端口无效", id)
				}
				serverPorts = []int{defaultNodePort}
			} else {
				serverPorts, err = parseFluxPortSpec(effectiveListenSpec, maxFluxExpandedPorts)
				if err != nil {
					return err
				}
			}

			transport, httpMode, err := normalizeFluxTransport(
				firstText(ch, "transport", "network"),
				firstText(ch, "httpMode", "http_mode"),
			)
			if err != nil {
				return fmt.Errorf("通道 %s：%w", id, err)
			}
			outCh["transport"] = transport

			httpHost := ""
			httpPath := "/"
			var httpHeaders map[string]string
			if transport == "http" {
				outCh["httpMode"] = httpMode
				if h := firstText(ch, "httpHost", "http_host"); h != "" {
					httpHost = h
					outCh["httpHost"] = h
				}
				if pth := firstText(ch, "httpPath", "http_path", "path"); pth != "" {
					if !strings.HasPrefix(pth, "/") {
						pth = "/" + pth
					}
					httpPath = pth
				}
				outCh["httpPath"] = httpPath
				rawHdr := ch["httpHeaders"]
				if rawHdr == nil {
					rawHdr = ch["http_headers"]
				}
				httpHeaders = normalizeHeadersMap(rawHdr)
				if len(httpHeaders) > 0 {
					outCh["httpHeaders"] = httpHeaders
				}
			}

			var chSec string
			if rawChSec := firstText(ch, "security"); rawChSec != "" && rawChSec != "inherit" {
				chSec, err = normalizeFluxSecurity(rawChSec)
				if err != nil {
					return fmt.Errorf("通道 %s：%w", id, err)
				}
				outCh["security"] = chSec
			}
			effSec := chSec
			if effSec == "" {
				effSec = defaultSec
			}

			nestedReality, _ := ch["reality"].(map[string]any)
			if nestedReality == nil {
				nestedReality, _ = ch["realityOpts"].(map[string]any)
			}
			if nestedReality == nil {
				nestedReality, _ = ch["reality_opts"].(map[string]any)
			}

			chSNI := firstText(ch, "sni", "serverName", "server_name")
			if chSNI == "" && nestedReality != nil {
				chSNI = firstText(nestedReality, "serverName", "server_name")
			}
			if chFP := strings.ToLower(firstText(ch, "fingerprint", "clientFingerprint", "client_fingerprint")); chFP != "" && chFP != "inherit" {
				if !c.Allowed(chFP, "chrome", "firefox", "safari", "ios", "edge", "random", "android", "360", "qq") {
					return fmt.Errorf("通道 %s 客户端指纹不支持：%s", id, chFP)
				}
				outCh["fingerprint"] = chFP
			}

			sigSecurity := effSec
			switch effSec {
			case "reality":
				chPriv := firstText(ch, "privateKey", "private_key")
				if chPriv == "" && nestedReality != nil {
					chPriv = firstText(nestedReality, "privateKey", "private_key")
				}
				chPub := firstText(ch, "publicKey", "public_key")
				if chPub == "" && nestedReality != nil {
					chPub = firstText(nestedReality, "publicKey", "public_key")
				}
				chShortID := firstText(ch, "shortId", "short_id")
				if chShortID == "" && nestedReality != nil {
					chShortID = firstText(nestedReality, "shortId", "short_id")
				}
				chDest := firstText(ch, "dest")
				if chDest == "" && nestedReality != nil {
					chDest = firstText(nestedReality, "dest")
				}
				if err := validateFluxShortID(chShortID); err != nil {
					return fmt.Errorf("通道 %s：%w", id, err)
				}
				if err := validateRealityDest(chDest); err != nil {
					return fmt.Errorf("通道 %s：%w", id, err)
				}
				if chPriv != "" {
					derived, err := deriveRealityPublicKey(chPriv)
					if err != nil {
						return fmt.Errorf("通道 %s：%w", id, err)
					}
					chPub = derived
				} else if nodePriv == "" {
					genPriv, genPub, err := generateRealityKeypair()
					if err != nil {
						return fmt.Errorf("通道 %s 生成 REALITY 密钥失败：%w", id, err)
					}
					chPriv, chPub = genPriv, genPub
					if chShortID == "" {
						chShortID, _ = generateFluxShortID()
					}
				}
				if chSNI == "" && chDest == "" && nodeRealitySNI == "" && p.Text(b, "serverName") == "" {
					chSNI = "www.microsoft.com"
				}
				if chSNI != "" {
					outCh["sni"] = chSNI
				}
				if chDest != "" {
					outCh["dest"] = chDest
				}
				if chPriv != "" {
					outCh["privateKey"] = chPriv
				}
				if chPub != "" {
					outCh["publicKey"] = chPub
				}
				if chShortID != "" {
					outCh["shortId"] = chShortID
				}
				effPriv := chPriv
				if effPriv == "" {
					effPriv = nodePriv
				}
				effSNI := chSNI
				if effSNI == "" {
					effSNI = nodeRealitySNI
				}
				effSID := chShortID
				if effSID == "" {
					effSID = nodeShortID
				}
				sigSecurity = fmt.Sprintf("reality:%s:%s:%s:%s", effPriv, effSNI, chDest, effSID)
			case "tls":
				rawCertMode := strings.ToLower(firstText(ch, "certMode", "cert_mode"))
				certMode := "inherit"
				switch rawCertMode {
				case "", "inherit":
					certMode = "inherit"
				case "file", "path":
					certMode = "file"
				case "origin_plain", "origin-plain", "none", "plain":
					certMode = "origin_plain"
				default:
					return fmt.Errorf("通道 %s 证书模式无效：%s", id, rawCertMode)
				}
				if certMode == "origin_plain" && transport == "udp" {
					return fmt.Errorf("通道 %s：UDP 通道不支持前置反代明文回源模式", id)
				}
				outCh["certMode"] = certMode
				if chSNI != "" {
					outCh["sni"] = chSNI
				}
				if p.Bool(p.Record(ch), "insecure") || p.Bool(p.Record(ch), "allow_insecure") {
					outCh["insecure"] = true
				}
				if certMode == "file" {
					certPath := firstText(ch, "certPath", "cert_path")
					keyPath := firstText(ch, "keyPath", "key_path")
					if certPath == "" || keyPath == "" {
						return fmt.Errorf("通道 %s 使用本地文件证书时必须填写证书路径和私钥路径", id)
					}
					outCh["certPath"] = certPath
					outCh["keyPath"] = keyPath
					sigSecurity = fmt.Sprintf("tls:file:%s:%s", certPath, keyPath)
				} else if certMode == "inherit" {
					if enabled {
						needsInheritedCert = true
					}
					sigSecurity = "tls:inherit"
				} else {
					sigSecurity = "tls:origin_plain"
				}
			case "obfs":
				chObfs := firstText(ch, "obfsKey", "obfs_key")
				if chObfs == "" && nodeObfsKey == "" {
					genObfs, err := generateFluxObfsKey()
					if err != nil {
						return fmt.Errorf("通道 %s 生成混淆密钥失败：%w", id, err)
					}
					chObfs = genObfs
				}
				if chObfs != "" {
					if len(chObfs) < 4 {
						return fmt.Errorf("通道 %s 混淆密钥长度至少为 4 位", id)
					}
					outCh["obfsKey"] = chObfs
				}
				effObfs := chObfs
				if effObfs == "" {
					effObfs = nodeObfsKey
				}
				sigSecurity = "obfs:" + effObfs
			}

			if enabled {
				isUDP := transport == "udp"
				sig := fmt.Sprintf("%s|%s|%s|%s|%s|%s", transport, httpMode, httpHost, httpPath, headersSignature(httpHeaders), sigSecurity)
				parsedListenIP := net.ParseIP(listenIP)
				isWildcard := parsedListenIP != nil && parsedListenIP.IsUnspecified()
				for _, port := range serverPorts {
					for otherKey, _ := range seenBindings {
						if otherKey.port == port && otherKey.isUDP == isUDP && otherKey.ip != listenIP {
							otherParsed := net.ParseIP(otherKey.ip)
							otherWildcard := otherParsed != nil && otherParsed.IsUnspecified()
							if isWildcard || otherWildcard {
								return fmt.Errorf("通道 %s 在本地端口 %d 上存在重叠监听地址（%s 与 %s）", id, port, otherKey.ip, listenIP)
							}
						}
					}
					bk := fluxBindKey{ip: listenIP, port: port, isUDP: isUDP}
					if existingSig, exists := seenBindings[bk]; exists {
						if existingSig != sig {
							return fmt.Errorf("通道 %s 在本地端口 %s:%d 上与其他通道的传输或安全配置冲突", id, listenIP, port)
						}
						continue
					}
					distinctServerPorts++
					if distinctServerPorts > maxFluxExpandedPorts {
						return fmt.Errorf("Flux 节点监听端口展开总数不能超过 %d", maxFluxExpandedPorts)
					}
					seenBindings[bk] = sig
				}
			}

			normalizedChannels = append(normalizedChannels, outCh)
		}

		if enabledCount == 0 {
			return fmt.Errorf("Flux 节点至少需要保留一条启用的通道")
		}
		options["channels"] = normalizedChannels
	}

	b["protocolOptions"] = options
	b["transport"] = "tcp"
	switch defaultSec {
	case "reality":
		b["tls"], b["tlsMode"] = true, "reality"
	case "tls":
		b["tls"], b["tlsMode"] = true, "tls"
	default:
		b["tls"], b["tlsMode"] = false, "none"
	}

	if needsInheritedCert {
		return validateNodeCertificate(b, creating, certificateChanged)
	}
	return nil
}

func fluxMachineSecurityAndOptions(n p.Record) (p.Record, map[string]any) {
	rawOpts, _ := n["protocolOptions"].(map[string]any)
	options := cloneMap(rawOpts)
	if options == nil {
		options = map[string]any{}
	}
	defaultSec := strings.ToLower(firstText(options, "security", "defaultSecurity", "default_security"))
	if defaultSec != "reality" && defaultSec != "tls" && defaultSec != "obfs" {
		switch p.Text(n, "tlsMode") {
		case "reality":
			defaultSec = "reality"
		case "tls":
			defaultSec = "tls"
		default:
			defaultSec = "reality"
		}
	}
	options["security"] = defaultSec
	for _, k := range []string{"defaultSecurity", "default_security", "ipStrategy", "clientFingerprint", "obfsKey"} {
		if s, ok := options[k].(string); ok && strings.TrimSpace(s) == "" {
			delete(options, k)
		}
	}
	if rawChannels, ok := options["channels"]; ok {
		cleaned := normalizeChannelSlice(rawChannels)
		for _, item := range cleaned {
			ch, _ := item.(map[string]any)
			for _, optEnum := range []string{"security", "certMode", "cert_mode", "httpMode", "http_mode", "role", "fingerprint", "transport"} {
				if s, ok := ch[optEnum].(string); ok && (strings.TrimSpace(s) == "" || s == "inherit") && optEnum != "certMode" && optEnum != "cert_mode" {
					delete(ch, optEnum)
				} else if s, ok := ch[optEnum].(string); ok && strings.TrimSpace(s) == "" {
					delete(ch, optEnum)
				}
			}
		}
		options["channels"] = cleaned
	}

	mode := "none"
	switch defaultSec {
	case "reality":
		mode = "reality"
	case "tls":
		mode = "tls"
	}
	security := p.Record{
		"mode":        mode,
		"serverName":  n["serverName"],
		"tlsSettings": n["tlsSettings"],
		"reality":     n["realitySettings"],
		"certificate": n["certConfig"],
	}
	if security["certificate"] == nil && mode == "tls" && (p.Text(n, "certPath") != "" || p.Text(n, "keyPath") != "") {
		security["certificate"] = p.Record{"mode": "file", "cert_path": n["certPath"], "key_path": n["keyPath"]}
	}
	return security, options
}

func sanitizeFluxSubscriptionNode(row p.Record) p.Record {
	out := p.Record{}
	for k, v := range row {
		switch k {
		case "certConfig", "serverPort", "certPath", "keyPath", "agentToken":
			continue
		case "realitySettings":
			if rs, ok := v.(map[string]any); ok {
				out[k] = sanitizeFluxRealityForClient(rs)
			} else if rs, ok := v.(p.Record); ok {
				out[k] = sanitizeFluxRealityForClient(map[string]any(rs))
			}
		case "protocolOptions":
			if opts, ok := v.(map[string]any); ok {
				out[k] = sanitizeFluxOptionsForClient(opts)
			} else if opts, ok := v.(p.Record); ok {
				out[k] = sanitizeFluxOptionsForClient(map[string]any(opts))
			}
		default:
			out[k] = cloneAny(v)
		}
	}
	return out
}

func sanitizeFluxRealityForClient(rs map[string]any) map[string]any {
	clean := map[string]any{}
	sni := firstText(rs, "serverName", "server_name")
	pub := firstText(rs, "publicKey", "public_key")
	if pub == "" {
		if priv := firstText(rs, "privateKey", "private_key"); priv != "" {
			if derived, err := deriveRealityPublicKey(priv); err == nil {
				pub = derived
			}
		}
	}
	sid := firstText(rs, "shortId", "short_id")
	if sni != "" {
		clean["serverName"] = sni
	}
	if pub != "" {
		clean["publicKey"] = pub
	}
	if sid != "" {
		clean["shortId"] = sid
	}
	return clean
}

func sanitizeFluxOptionsForClient(opts map[string]any) map[string]any {
	clean := map[string]any{}
	for _, k := range []string{"security", "ipStrategy", "ip_strategy", "standbyChannels", "standby_channels", "rotationIntervalSec", "rotation_interval_sec", "rotationBytesMb", "rotation_bytes_mb", "clientFingerprint", "client_fingerprint", "obfsKey", "obfs_key", "alpn", "udp"} {
		if val, exists := opts[k]; exists && val != nil && val != "" {
			clean[k] = cloneAny(val)
		}
	}
	if rawReality, ok := opts["reality"].(map[string]any); ok {
		if cr := sanitizeFluxRealityForClient(rawReality); len(cr) > 0 {
			clean["reality"] = cr
		}
	} else if rawReality, ok := opts["reality"].(p.Record); ok {
		if cr := sanitizeFluxRealityForClient(map[string]any(rawReality)); len(cr) > 0 {
			clean["reality"] = cr
		}
	}
	if rawChannels, hasChannels := opts["channels"]; hasChannels {
		items := normalizeChannelSlice(rawChannels)
		pubChannels := make([]any, 0, len(items))
		for _, item := range items {
			ch, ok := item.(map[string]any)
			if !ok || !boolTrueDefault(ch, "enabled") {
				continue
			}
			pubCh := map[string]any{"enabled": true}
			for _, k := range []string{"id", "connectHost", "connect_host", "server", "connectPorts", "connect_ports", "ports", "ipVersion", "ip_version", "transport", "httpMode", "http_mode", "httpHost", "http_host", "httpPath", "http_path", "httpHeaders", "http_headers", "security", "sni", "alpn", "fingerprint", "clientFingerprint", "client_fingerprint", "insecure", "allow_insecure", "obfsKey", "obfs_key", "role", "weight"} {
				if val, exists := ch[k]; exists && val != nil && val != "" {
					pubCh[k] = cloneAny(val)
				}
			}
			pub := firstText(ch, "publicKey", "public_key")
			sid := firstText(ch, "shortId", "short_id")
			priv := firstText(ch, "privateKey", "private_key")
			if nested, ok := ch["reality"].(map[string]any); ok {
				if pub == "" {
					pub = firstText(nested, "publicKey", "public_key")
				}
				if sid == "" {
					sid = firstText(nested, "shortId", "short_id")
				}
				if priv == "" {
					priv = firstText(nested, "privateKey", "private_key")
				}
				if _, hasSNI := pubCh["sni"]; !hasSNI {
					if nSNI := firstText(nested, "serverName", "server_name"); nSNI != "" {
						pubCh["sni"] = nSNI
					}
				}
			}
			if pub == "" && priv != "" {
				if derived, err := deriveRealityPublicKey(priv); err == nil {
					pub = derived
				}
			}
			if pub != "" {
				pubCh["publicKey"] = pub
			}
			if sid != "" {
				pubCh["shortId"] = sid
			}
			pubChannels = append(pubChannels, pubCh)
		}
		clean["channels"] = pubChannels
	}
	return clean
}

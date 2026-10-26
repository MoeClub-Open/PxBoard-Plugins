package nodes

import (
	"fmt"
	"net"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func mergeNodeSettings(b, old p.Record) {
	for _, key := range []string{"certConfig", "protocolOptions"} {
		patch, ok := b[key].(map[string]any)
		if !ok {
			continue
		}
		previous, _ := old[key].(map[string]any)
		b[key] = mergeObject(previous, patch)
		if key == "certConfig" {
			merged := b[key].(map[string]any)
			for canonical, aliases := range map[string][]string{
				"mode":      {"cert_mode", "certMode"},
				"cert_path": {"certificate_path", "certificatePath", "fullchain_path", "fullchainPath", "fullchain"},
				"key_path":  {"private_key_path", "privateKeyPath", "privkey_path", "privkeyPath", "privkey"},
			} {
				if _, changed := patch[canonical]; changed {
					for _, alias := range aliases {
						delete(merged, alias)
					}
				}
			}
		}
	}
}

func mergeObject(old, patch map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range old {
		out[key] = value
	}
	for key, value := range patch {
		if nested, ok := value.(map[string]any); ok {
			previous, _ := old[key].(map[string]any)
			value = mergeObject(previous, nested)
		}
		out[key] = value
	}
	return out
}

func normalizeNodeConnection(b p.Record, creating, transportChanged, certificateChanged bool) error {
	if b["serverPort"] == "" {
		b["serverPort"] = nil
	}
	if port := p.Number(b, "serverPort"); port < 0 || port > 65535 {
		return fmt.Errorf("节点监听端口无效")
	}
	if p.Text(b, "engine") != "noders" {
		return nil
	}
	switch p.Text(b, "protocol") {
	case "anytls":
		b["tls"], b["tlsMode"], b["transport"] = true, "tls", "tcp"
	case "mieru":
		b["tls"], b["tlsMode"] = false, "none"
		transport := strings.ToLower(p.Text(b, "transport"))
		if transport == "" {
			transport = "tcp"
		}
		if transport != "tcp" && transport != "udp" {
			return fmt.Errorf("Mieru 传输方式请选择 TCP 或 UDP")
		}
		b["transport"] = transport
		if options, ok := b["protocolOptions"].(map[string]any); ok && transportChanged {
			options = mergeObject(options, nil)
			if previous, exists := options["transport"]; exists {
				if nested, ok := previous.(map[string]any); ok {
					options["transport"] = mergeObject(nested, map[string]any{"type": transport})
				} else {
					options["transport"] = transport
				}
			}
			b["protocolOptions"] = options
		}
		return nil
	case "trojan", "hysteria2", "tuic", "naive":
		b["tls"] = true
	case "vless", "vmess":
		if !p.Bool(b, "tls") {
			b["tlsMode"] = "none"
		}
	}
	if !p.Bool(b, "tls") || p.Text(b, "tlsMode") == "reality" {
		return nil
	}
	cert, _ := b["certConfig"].(map[string]any)
	mode := firstText(cert, "mode", "cert_mode", "certMode")
	if mode == "" {
		return nil
	}
	if (creating || certificateChanged) && (mode == "file" || mode == "path") {
		if firstText(cert, "cert_path", "certificate_path", "certificatePath", "fullchain_path", "fullchainPath", "fullchain") == "" || firstText(cert, "key_path", "private_key_path", "privateKeyPath", "privkey_path", "privkeyPath", "privkey") == "" {
			return fmt.Errorf("请填写节点服务器上的证书和私钥路径")
		}
	}
	if (creating || certificateChanged) && (mode == "http" || mode == "acme" || mode == "letsencrypt" || mode == "dns") {
		domain := firstText(cert, "domain")
		if domain == "" {
			if names, ok := cert["domains"].([]any); ok && len(names) > 0 {
				domain, _ = names[0].(string)
			}
		}
		if domain == "" {
			cert["domain"] = firstText(b, "serverName", "host")
			domain = p.Text(cert, "domain")
		}
		domains := strings.FieldsFunc(domain, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' })
		if len(domains) == 0 || net.ParseIP(domains[0]) != nil {
			return fmt.Errorf("证书签发需要域名，请填写 TLS 域名或签发域名")
		}
		if p.Text(b, "serverName") == "" {
			b["serverName"] = domains[0]
		}
		if mode == "dns" {
			switch firstText(cert, "provider", "dns_provider") {
			case "cloudflare", "cf":
				if firstText(cert, "api_token") == "" {
					return fmt.Errorf("请填写 Cloudflare API 令牌")
				}
			case "alidns", "aliyun", "ali":
				if firstText(cert, "access_key_id") == "" || firstText(cert, "access_key_secret") == "" {
					return fmt.Errorf("请填写阿里云 AccessKey ID 和 Secret")
				}
			default:
				return fmt.Errorf("请选择 DNS 服务商")
			}
		}
	}
	return nil
}

func firstText(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(p.Text(row, key)); value != "" {
			return value
		}
	}
	return ""
}

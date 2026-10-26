package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
)

func Config(n p.Record, clients, rules []p.Record, accessLog string) ([]byte, error) {
	if p.Text(n, "engine") == "sing-box" {
		return singbox(n, clients, rules)
	}
	return xray(n, clients, rules, accessLog)
}
func xray(n p.Record, clients, rules []p.Record, accessLog string) ([]byte, error) {
	proto := p.Text(n, "protocol")
	users := []any{}
	for _, c := range clients {
		cred := p.Text(c, "credential")
		u := p.Record{"email": c["userId"], "level": 0}
		switch proto {
		case "vless", "vmess":
			u["id"] = cred
		case "trojan", "shadowsocks":
			u["password"] = cred
			if proto == "shadowsocks" {
				u["method"] = "aes-128-gcm"
			}
		default:
			return nil, fmt.Errorf("Xray 不支持此协议")
		}
		users = append(users, u)
	}
	settings := p.Record{"clients": users}
	if proto == "vless" {
		settings["decryption"] = "none"
	}
	if proto == "shadowsocks" {
		settings["network"] = "tcp,udp"
		settings["method"] = "aes-128-gcm"
	}
	stream := p.Record{"network": "tcp"}
	if p.Bool(n, "tls") {
		if p.Text(n, "certPath") == "" || p.Text(n, "keyPath") == "" {
			return nil, fmt.Errorf("启用 TLS 需要配置服务器证书和私钥路径")
		}
		stream["security"] = "tls"
		stream["tlsSettings"] = p.Record{"certificates": []any{p.Record{"certificateFile": n["certPath"], "keyFile": n["keyPath"]}}}
	}
	routes := []any{p.Record{"type": "field", "inboundTag": []string{"api"}, "outboundTag": "api"}}
	for _, rule := range rules {
		r := p.Record{"type": "field", "inboundTag": []string{"proxy"}, "outboundTag": "block"}
		pattern := p.Text(rule, "pattern")
		switch p.Text(rule, "kind") {
		case "domain":
			r["domain"] = []string{"domain:" + pattern}
		case "keyword":
			r["domain"] = []string{pattern}
		case "regex":
			r["domain"] = []string{"regexp:" + pattern}
		case "cidr":
			r["ip"] = []string{pattern}
		case "protocol":
			r["protocol"] = []string{pattern}
		default:
			continue
		}
		routes = append(routes, r)
	}
	logs := p.Record{"loglevel": "warning"}
	if accessLog != "" {
		logs["access"] = accessLog
	}
	config := p.Record{"log": logs, "stats": p.Record{}, "api": p.Record{"tag": "api", "services": []string{"StatsService"}}, "policy": p.Record{"levels": p.Record{"0": p.Record{"statsUserUplink": true, "statsUserDownlink": true}}}, "inbounds": []any{p.Record{"tag": "api", "listen": "127.0.0.1", "port": 10085, "protocol": "dokodemo-door", "settings": p.Record{"address": "127.0.0.1"}}, p.Record{"tag": "proxy", "listen": "0.0.0.0", "port": n["port"], "protocol": proto, "settings": settings, "streamSettings": stream, "sniffing": p.Record{"enabled": true, "destOverride": []string{"http", "tls", "quic"}, "routeOnly": true}}}, "outbounds": []any{p.Record{"tag": "direct", "protocol": "freedom"}, p.Record{"tag": "block", "protocol": "blackhole"}}, "routing": p.Record{"rules": routes}}
	return json.MarshalIndent(config, "", "  ")
}
func singbox(n p.Record, clients, rules []p.Record) ([]byte, error) {
	proto := p.Text(n, "protocol")
	users := []any{}
	names := []string{}
	for _, c := range clients {
		u := p.Record{"name": c["userId"]}
		names = append(names, p.Text(c, "userId"))
		switch proto {
		case "vless", "vmess":
			u["uuid"] = c["credential"]
		case "trojan", "hysteria2", "shadowsocks":
			u["password"] = c["credential"]
		default:
			return nil, fmt.Errorf("sing-box 协议无效")
		}
		users = append(users, u)
	}
	if proto == "shadowsocks" {
		return nil, fmt.Errorf("sing-box 多用户 Shadowsocks 需要 2022 密钥，目前请使用 Xray")
	}
	inbound := p.Record{"type": proto, "tag": "proxy", "listen": "::", "listen_port": n["port"], "users": users}
	if p.Bool(n, "tls") {
		if p.Text(n, "certPath") == "" || p.Text(n, "keyPath") == "" {
			return nil, fmt.Errorf("请配置 TLS 证书路径")
		}
		inbound["tls"] = p.Record{"enabled": true, "server_name": n["serverName"], "certificate_path": n["certPath"], "key_path": n["keyPath"]}
	}
	routes := []any{}
	for _, rule := range rules {
		r := p.Record{"action": "reject"}
		pattern := p.Text(rule, "pattern")
		switch p.Text(rule, "kind") {
		case "domain":
			r["domain_suffix"] = []string{pattern}
		case "keyword":
			r["domain_keyword"] = []string{pattern}
		case "regex":
			r["domain_regex"] = []string{pattern}
		case "cidr":
			r["ip_cidr"] = []string{pattern}
		case "protocol":
			r["protocol"] = []string{pattern}
		default:
			continue
		}
		routes = append(routes, r)
	}
	return json.MarshalIndent(p.Record{"log": p.Record{"level": "warn"}, "inbounds": []any{inbound}, "outbounds": []any{p.Record{"type": "direct", "tag": "direct"}}, "route": p.Record{"rules": routes, "final": "direct"}, "experimental": p.Record{"v2ray_api": p.Record{"listen": "127.0.0.1:10085", "stats": p.Record{"enabled": true, "users": names}}}}, "", "  ")
}

var accessPattern = regexp.MustCompile(`(?:accepted|rejected)\s+(?:tcp:|udp:)?([^\s]+).*email:\s*([^\s]+)`)

func ReadFlows(path string, offset int64) ([]p.Record, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, offset, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, offset, e
	}
	if offset < 0 || offset > stat.Size() {
		offset = stat.Size()
		return []p.Record{}, offset, nil
	}
	if _, e = f.Seek(offset, io.SeekStart); e != nil {
		return nil, offset, e
	}
	reader := bufio.NewReader(f)
	out := []p.Record{}
	for len(out) < 1000 {
		line, e := reader.ReadString('\n')
		if e != nil {
			if e == io.EOF {
				break
			}
			return nil, offset, e
		}
		offset += int64(len(line))
		m := accessPattern.FindStringSubmatch(line)
		if m != nil {
			host := m[1]
			if i := strings.LastIndex(host, ":"); i >= 0 {
				host = host[:i]
			}
			host = strings.Trim(host, "[]")
			proto := "tcp"
			if strings.Contains(line, "udp:") {
				proto = "udp"
			}
			flow := p.Record{"host": host, "userId": m[2], "protocol": proto}
			if ip := net.ParseIP(host); ip != nil {
				flow["ip"] = ip.String()
			}
			out = append(out, flow)
		}
	}
	return out, offset, nil
}

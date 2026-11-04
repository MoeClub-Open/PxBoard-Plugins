package subscriptions

import (
	"encoding/base64"
	"encoding/json"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/url"
	"strconv"
	"strings"
)

func subscriptionPassword(account, n p.Record) string {
	password := c.PasswordFor(account, n)
	options, _ := n["protocolOptions"].(map[string]any)
	if p.Text(n, "protocol") == "shadowsocks" && strings.HasPrefix(cipher(n), "2022-") {
		key := p.Text(options, "serverKey")
		if key == "" {
			key = p.Text(options, "server_key")
		}
		if key != "" {
			password = key + ":" + password
		}
	}
	return password
}
func cipher(n p.Record) string {
	options, _ := n["protocolOptions"].(map[string]any)
	value := p.Text(options, "cipher")
	if value == "" {
		value = p.Text(options, "method")
	}
	if value == "" {
		value = "aes-128-gcm"
	}
	return value
}
func clash(n p.Record, cred, uuid string) p.Record {
	proto := p.Text(n, "protocol")
	if proto == "mieru" {
		return mieruClash(n, uuid)
	}
	typ := proto
	if proto == "shadowsocks" {
		typ = "ss"
	}
	out := p.Record{"name": n["name"], "type": typ, "server": n["host"], "port": n["port"], "udp": true}
	switch proto {
	case "vless", "vmess":
		out["uuid"] = uuid
		if proto == "vmess" {
			out["alterId"] = 0
			out["cipher"] = "auto"
		}
	case "shadowsocks":
		out["cipher"] = cipher(n)
		out["password"] = cred
	default:
		out["password"] = cred
	}
	if proto == "tuic" {
		out["uuid"] = uuid
	}
	options, _ := n["protocolOptions"].(map[string]any)
	if flow := p.Text(options, "flow"); flow != "" {
		out["flow"] = flow
	}
	clashSecurity(n, out)
	clashTransport(n, out)
	clashProtocolOptions(n, out)
	return out
}
func singbox(n p.Record, cred, uuid string) p.Record {
	proto := p.Text(n, "protocol")
	out := p.Record{"tag": n["name"], "type": proto, "server": n["host"], "server_port": n["port"]}
	switch proto {
	case "vless", "vmess":
		out["uuid"] = uuid
		if proto == "vmess" {
			out["security"] = "auto"
		}
	case "shadowsocks":
		out["method"] = cipher(n)
		out["password"] = cred
	default:
		out["password"] = cred
	}
	if proto == "tuic" {
		out["uuid"] = uuid
	}
	if flow := p.Text(object(n, "protocolOptions"), "flow"); flow != "" {
		out["flow"] = flow
	}
	singboxSecurity(n, out)
	singboxTransport(n, out)
	singboxProtocolOptions(n, out)
	return out
}
func uri(n p.Record, cred, uuid string) string {
	proto := p.Text(n, "protocol")
	if proto == "mieru" {
		return mieruURI(n, uuid)
	}
	host := p.Text(n, "host")
	if strings.Contains(host, ":") {
		host = "[" + strings.Trim(host, "[]") + "]"
	}
	port := strconv.Itoa(int(p.Number(n, "port")))
	q := url.Values{}
	uriSecurity(n, q)
	uriTransport(n, q)
	uriProtocolOptions(n, q)
	switch proto {
	case "vmess":
		settings := object(n, "transportSettings")
		v := p.Record{"v": "2", "ps": n["name"], "add": n["host"], "port": port, "id": uuid, "aid": "0", "scy": "auto", "net": network(n), "tls": "", "host": transportHost(settings), "path": settings["path"]}
		if network(n) == "grpc" {
			v["path"] = settings["serviceName"]
		}
		if p.Bool(n, "tls") {
			v["tls"] = "tls"
			v["sni"] = n["serverName"]
			if p.Bool(tlsSettings(n), "allow_insecure") {
				v["allowInsecure"] = true
			}
		}
		b, _ := json.Marshal(v)
		return "vmess://" + base64.StdEncoding.EncodeToString(b)
	case "shadowsocks":
		return "ss://" + base64.RawURLEncoding.EncodeToString([]byte(cipher(n)+":"+cred)) + "@" + host + ":" + port + "#" + url.PathEscape(p.Text(n, "name"))
	default:
		identity := url.User(cred).String()
		if proto == "vless" {
			identity = url.User(uuid).String()
		}
		if proto == "tuic" {
			identity = url.UserPassword(uuid, cred).String()
		}
		return proto + "://" + identity + "@" + host + ":" + port + "?" + q.Encode() + "#" + url.PathEscape(p.Text(n, "name"))
	}
}

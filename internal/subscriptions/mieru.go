package subscriptions

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/url"
	"strconv"
	"strings"
)

func mieruTransport(n p.Record) string {
	options, _ := n["protocolOptions"].(map[string]any)
	transport := p.Text(options, "transport")
	if object, ok := options["transport"].(map[string]any); ok {
		transport = p.Text(object, "type")
	}
	if transport == "" {
		transport = p.Text(n, "transport")
	}
	if strings.EqualFold(transport, "udp") {
		return "UDP"
	}
	return "TCP"
}

func mieruOptions(n p.Record) p.Record {
	options, _ := n["protocolOptions"].(map[string]any)
	out := p.Record{}
	for _, key := range []string{"multiplexing", "handshake-mode", "traffic-pattern"} {
		value := p.Text(options, key)
		if value == "" {
			value = p.Text(options, strings.ReplaceAll(key, "-", "_"))
		}
		if value != "" {
			out[key] = value
		}
	}
	return out
}

func mieruClash(n p.Record, uuid string) p.Record {
	out := mieruOptions(n)
	for key, value := range (p.Record{"name": n["name"], "type": "mieru", "server": n["host"], "port": n["port"], "username": uuid, "password": uuid, "transport": mieruTransport(n), "udp": true}) {
		out[key] = value
	}
	return out
}

func mieruURI(n p.Record, uuid string) string {
	q := url.Values{"profile": {p.Text(n, "name")}, "port": {strconv.Itoa(int(p.Number(n, "port")))}, "protocol": {mieruTransport(n)}}
	for key, value := range mieruOptions(n) {
		q.Set(key, value.(string))
	}
	host := p.Text(n, "host")
	if strings.Contains(host, ":") {
		host = "[" + strings.Trim(host, "[]") + "]"
	}
	return (&url.URL{Scheme: "mierus", User: url.UserPassword(uuid, uuid), Host: host, RawQuery: q.Encode()}).String()
}

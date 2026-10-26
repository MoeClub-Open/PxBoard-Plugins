package subscriptions

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/url"
)

func network(n p.Record) string {
	value := p.Text(n, "transport")
	switch value {
	case "", "raw":
		return "tcp"
	case "websocket":
		return "ws"
	case "splithttp":
		return "xhttp"
	case "http":
		return "h2"
	}
	return value
}

func streamProtocol(n p.Record) bool {
	switch p.Text(n, "protocol") {
	case "vmess", "vless", "trojan":
		return true
	}
	return false
}

func transportHost(settings p.Record) string {
	if host := p.Text(settings, "host"); host != "" {
		return host
	}
	if host := p.Text(object(settings, "headers"), "Host"); host != "" {
		return host
	}
	if hosts, ok := settings["host"].([]any); ok && len(hosts) > 0 {
		host, _ := hosts[0].(string)
		return host
	}
	return ""
}

func clashTransport(n, out p.Record) {
	if !streamProtocol(n) {
		return
	}
	kind, settings := network(n), object(n, "transportSettings")
	out["network"] = kind
	switch kind {
	case "ws", "httpupgrade":
		out["network"] = "ws"
		options := p.Record{"path": settings["path"]}
		if settings["headers"] != nil {
			options["headers"] = settings["headers"]
		} else if host := transportHost(settings); host != "" {
			options["headers"] = p.Record{"Host": host}
		}
		if kind == "httpupgrade" {
			options["v2ray-http-upgrade"] = true
		}
		out["ws-opts"] = options
	case "grpc":
		out["grpc-opts"] = p.Record{"grpc-service-name": settings["serviceName"]}
	case "h2":
		options := p.Record{"path": settings["path"]}
		if hosts, ok := settings["host"].([]any); ok {
			options["host"] = hosts
		} else if host := transportHost(settings); host != "" {
			options["host"] = []string{host}
		}
		out["h2-opts"] = options
	case "xhttp":
		options := p.Record{}
		for _, key := range []string{"path", "host", "mode", "extra"} {
			if settings[key] != nil {
				options[key] = settings[key]
			}
		}
		out["xhttp-opts"] = options
	}
}

func singboxTransport(n, out p.Record) {
	if !streamProtocol(n) || network(n) == "tcp" {
		return
	}
	kind, settings := network(n), object(n, "transportSettings")
	transport := p.Record{"type": kind}
	switch kind {
	case "grpc":
		transport["service_name"] = settings["serviceName"]
	case "ws":
		transport["path"] = settings["path"]
		if settings["headers"] != nil {
			transport["headers"] = settings["headers"]
		} else if host := transportHost(settings); host != "" {
			transport["headers"] = p.Record{"Host": host}
		}
	case "httpupgrade":
		transport["path"], transport["host"] = settings["path"], transportHost(settings)
	case "h2":
		transport["type"], transport["path"] = "http", settings["path"]
		if hosts, ok := settings["host"].([]any); ok {
			transport["host"] = hosts
		} else if host := transportHost(settings); host != "" {
			transport["host"] = []string{host}
		}
	}
	out["transport"] = transport
}

func uriTransport(n p.Record, q url.Values) {
	if !streamProtocol(n) {
		return
	}
	kind, settings := network(n), object(n, "transportSettings")
	q.Set("type", kind)
	if kind == "grpc" {
		q.Set("serviceName", p.Text(settings, "serviceName"))
		return
	}
	if host := transportHost(settings); host != "" {
		q.Set("host", host)
	}
	if path := p.Text(settings, "path"); path != "" {
		q.Set("path", path)
	}
	if mode := p.Text(settings, "mode"); mode != "" {
		q.Set("mode", mode)
	}
}

func supportedFormat(n p.Record, format string) bool {
	protocol := p.Text(n, "protocol")
	if protocol == "mieru" {
		return format != "sing-box"
	}
	switch protocol {
	case "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "anytls", "tuic":
	default:
		return false
	}
	return format != "sing-box" || !streamProtocol(n) || network(n) != "xhttp"
}

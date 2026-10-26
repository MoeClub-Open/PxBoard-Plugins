package subscriptions

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/url"
	"strings"
)

func object(row p.Record, key string) p.Record {
	value, _ := row[key].(map[string]any)
	return value
}

func tlsSettings(n p.Record) p.Record {
	if p.Text(n, "tlsMode") == "reality" {
		return object(n, "realitySettings")
	}
	return object(n, "tlsSettings")
}

func fingerprint(n p.Record) string {
	options := object(n, "protocolOptions")
	utls := object(options, "utls")
	value := p.Text(utls, "fingerprint")
	if value == "" {
		value = p.Text(options, "client_fingerprint")
	}
	if value == "" && p.Text(n, "tlsMode") == "reality" {
		value = "chrome"
	}
	return value
}

func clashSecurity(n, out p.Record) {
	if !p.Bool(n, "tls") {
		return
	}
	settings := tlsSettings(n)
	out["tls"], out["servername"], out["sni"] = true, n["serverName"], n["serverName"]
	if p.Bool(settings, "allow_insecure") {
		out["skip-cert-verify"] = true
	}
	if value := fingerprint(n); value != "" {
		out["client-fingerprint"] = value
	}
	if alpn := settings["alpn"]; alpn != nil {
		out["alpn"] = alpn
	} else if alpn := object(n, "protocolOptions")["alpn"]; alpn != nil {
		out["alpn"] = alpn
	}
	if p.Text(n, "tlsMode") == "reality" {
		out["reality-opts"] = p.Record{"public-key": settings["public_key"], "short-id": settings["short_id"]}
		return
	}
	ech := object(settings, "ech")
	if p.Bool(ech, "enabled") {
		value := strings.NewReplacer("-----BEGIN ECHCONFIG-----", "", "-----END ECHCONFIG-----", "\r", "", "\n", "").Replace(p.Text(ech, "config"))
		config := p.Record{"enable": true}
		if value != "" {
			config["config"] = strings.TrimSpace(value)
		}
		if value := p.Text(ech, "query_server_name"); value != "" {
			config["query-server-name"] = value
		}
		out["ech-opts"] = config
	}
}

func singboxSecurity(n, out p.Record) {
	if !p.Bool(n, "tls") {
		return
	}
	settings := tlsSettings(n)
	tls := p.Record{"enabled": true, "server_name": n["serverName"]}
	if p.Bool(settings, "allow_insecure") {
		tls["insecure"] = true
	}
	if value := fingerprint(n); value != "" {
		tls["utls"] = p.Record{"enabled": true, "fingerprint": value}
	}
	if alpn := settings["alpn"]; alpn != nil {
		tls["alpn"] = alpn
	} else if alpn := object(n, "protocolOptions")["alpn"]; alpn != nil {
		tls["alpn"] = alpn
	}
	if p.Text(n, "tlsMode") == "reality" {
		tls["reality"] = p.Record{"enabled": true, "public_key": settings["public_key"], "short_id": settings["short_id"]}
	} else if ech := object(settings, "ech"); p.Bool(ech, "enabled") {
		config := p.Record{"enabled": true}
		if value := p.Text(ech, "config"); value != "" {
			config["config"] = []string{value}
		}
		if value := p.Text(ech, "query_server_name"); value != "" {
			config["query_server_name"] = value
		}
		tls["ech"] = config
	}
	out["tls"] = tls
}

func uriSecurity(n p.Record, q url.Values) {
	if !p.Bool(n, "tls") {
		return
	}
	settings := tlsSettings(n)
	q.Set("security", "tls")
	q.Set("sni", p.Text(n, "serverName"))
	if p.Bool(settings, "allow_insecure") {
		q.Set("allowInsecure", "1")
	}
	if value := fingerprint(n); value != "" {
		q.Set("fp", value)
	}
	if p.Text(n, "tlsMode") == "reality" {
		q.Set("security", "reality")
		q.Set("pbk", p.Text(settings, "public_key"))
		q.Set("sid", p.Text(settings, "short_id"))
	} else if ech := object(settings, "ech"); p.Bool(ech, "enabled") {
		if value := p.Text(ech, "config"); value != "" {
			q.Set("ech", value)
		}
	}
}

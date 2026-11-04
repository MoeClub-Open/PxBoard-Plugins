package subscriptions

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func supportsFlux(r *http.Request) bool {
	if r == nil {
		return false
	}
	hasExplicitTypes := false
	for _, rawTypes := range r.URL.Query()["types"] {
		for _, token := range strings.FieldsFunc(rawTypes, func(c rune) bool {
			return c == ',' || c == '|' || c == '｜' || c == ' '
		}) {
			t := strings.ToLower(strings.TrimSpace(token))
			if t == "" {
				continue
			}
			if t == "all" {
				continue
			}
			hasExplicitTypes = true
			if t == "flux" {
				return true
			}
		}
	}
	if hasExplicitTypes {
		return false
	}
	hasExplicitHeader := false
	for _, header := range r.Header.Values("X-XBClient-Protocols") {
		for _, token := range strings.Split(header, ",") {
			t := strings.ToLower(strings.TrimSpace(token))
			if t == "" {
				continue
			}
			hasExplicitHeader = true
			if t == "flux" {
				return true
			}
		}
	}
	if hasExplicitHeader {
		return false
	}
	flag := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("flag")))
	if strings.Contains(flag, "aerion") || strings.Contains(flag, "xbclient") {
		return true
	}
	ua := strings.ToLower(r.UserAgent())
	return strings.Contains(ua, "aerion") || strings.Contains(ua, "xbclient")
}

func fluxFirstText(m p.Record, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		if val := strings.TrimSpace(p.Text(m, k)); val != "" {
			return val
		}
	}
	return ""
}

func fluxInt(m p.Record, def int, keys ...string) int {
	for _, k := range keys {
		raw, exists := m[k]
		if !exists || raw == nil || raw == "" {
			continue
		}
		switch v := raw.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case int64:
			return int(v)
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				return parsed
			}
		}
	}
	return def
}

func fluxPortString(raw any) string {
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
			if s := fluxPortString(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func fluxCleanSecurity(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "reality":
		return "reality"
	case "tls":
		return "tls"
	case "obfs", "salamander":
		return "obfs"
	default:
		return ""
	}
}

func fluxCleanIpStrategy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "prefer_v4", "prefer-v4", "ipv4_first":
		return "prefer_v4"
	case "prefer_v6", "prefer-v6", "ipv6_first":
		return "prefer_v6"
	case "v4_only", "v4-only", "ipv4", "ipv4_only":
		return "v4_only"
	case "v6_only", "v6-only", "ipv6", "ipv6_only":
		return "v6_only"
	default:
		return "dual"
	}
}

func fluxCleanTransport(rawTransport, rawHttpMode string) (string, string) {
	t := strings.ToLower(strings.TrimSpace(rawTransport))
	m := strings.ToLower(strings.TrimSpace(rawHttpMode))
	switch t {
	case "udp", "quic", "h3":
		return "udp", ""
	case "ws", "websocket":
		return "http", "ws"
	case "httpupgrade", "http-upgrade", "http_upgrade", "upgrade":
		return "http", "httpupgrade"
	case "h2", "http2":
		return "http", "h2"
	case "http":
		switch m {
		case "httpupgrade", "http-upgrade", "http_upgrade", "upgrade":
			return "http", "httpupgrade"
		case "h2", "http2", "http":
			return "http", "h2"
		default:
			return "http", "ws"
		}
	default:
		return "tcp", ""
	}
}

func fluxHeaders(raw any) map[string]string {
	out := map[string]string{}
	switch typed := raw.(type) {
	case map[string]string:
		for k, v := range typed {
			if kt := strings.TrimSpace(k); kt != "" {
				out[kt] = strings.TrimSpace(v)
			}
		}
	case map[string]any:
		for k, v := range typed {
			if kt := strings.TrimSpace(k); kt != "" && v != nil {
				out[kt] = strings.TrimSpace(fmt.Sprint(v))
			}
		}
	case p.Record:
		for k, v := range typed {
			if kt := strings.TrimSpace(k); kt != "" && v != nil {
				out[kt] = strings.TrimSpace(fmt.Sprint(v))
			}
		}
	}
	return out
}

func fluxRecord(raw any) p.Record {
	switch v := raw.(type) {
	case p.Record:
		return v
	case map[string]any:
		return p.Record(v)
	default:
		return nil
	}
}

func fluxClash(n p.Record, cred, uuid string) p.Record {
	options := fluxRecord(n["protocolOptions"])
	key := strings.TrimSpace(uuid)
	if key == "" {
		key = strings.TrimSpace(cred)
	}
	out := p.Record{
		"name":   n["name"],
		"type":   "flux",
		"server": n["host"],
		"port":   int(p.Number(n, "port")),
		"key":    key,
		"udp":    true,
	}
	clashFluxProtocolOptions(n, options, out)
	return out
}

func clashFluxProtocolOptions(n, options, out p.Record) {
	if options == nil {
		options = p.Record{}
	}
	out["ip-strategy"] = fluxCleanIpStrategy(fluxFirstText(options, "ipStrategy", "ip_strategy"))
	out["standby-channels"] = max(0, min(16, fluxInt(options, 2, "standbyChannels", "standby_channels")))
	out["rotation-interval-sec"] = max(0, fluxInt(options, 180, "rotationIntervalSec", "rotation_interval_sec"))
	out["rotation-bytes-mb"] = max(0, fluxInt(options, 96, "rotationBytesMb", "rotation_bytes_mb"))

	realityOpts := fluxRecord(options["reality"])
	rs := fluxRecord(n["realitySettings"])
	nodePub := fluxFirstText(realityOpts, "publicKey", "public_key")
	if nodePub == "" {
		nodePub = fluxFirstText(rs, "publicKey", "public_key")
	}
	nodeSID := fluxFirstText(realityOpts, "shortId", "short_id")
	if nodeSID == "" {
		nodeSID = fluxFirstText(rs, "shortId", "short_id")
	}
	nodeRealitySNI := fluxFirstText(realityOpts, "serverName", "server_name")
	if nodeRealitySNI == "" {
		nodeRealitySNI = fluxFirstText(rs, "serverName", "server_name")
	}
	nodeObfs := fluxFirstText(options, "obfsKey", "obfs_key")

	defaultSec := fluxCleanSecurity(fluxFirstText(options, "security", "defaultSecurity", "default_security"))
	if defaultSec == "" {
		switch p.Text(n, "tlsMode") {
		case "reality":
			defaultSec = "reality"
		case "tls":
			defaultSec = "tls"
		default:
			if nodePub != "" {
				defaultSec = "reality"
			} else if nodeObfs != "" {
				defaultSec = "obfs"
			} else {
				defaultSec = "tls"
			}
		}
	}
	out["security"] = defaultSec

	fp := fluxFirstText(options, "clientFingerprint", "client_fingerprint")
	if fp == "" {
		fp = fingerprint(n)
	}
	if fp == "" && defaultSec == "reality" {
		fp = "chrome"
	}

	switch defaultSec {
	case "reality":
		sni := nodeRealitySNI
		if sni == "" {
			sni = strings.TrimSpace(p.Text(n, "serverName"))
		}
		if sni != "" {
			out["servername"] = sni
		}
		if fp != "" {
			out["client-fingerprint"] = fp
		}
		if nodePub != "" {
			out["reality-opts"] = p.Record{"public-key": nodePub, "short-id": nodeSID}
		}
	case "tls":
		if sni := strings.TrimSpace(p.Text(n, "serverName")); sni != "" {
			out["servername"] = sni
		}
		if fp != "" {
			out["client-fingerprint"] = fp
		}
		if p.Bool(fluxRecord(n["tlsSettings"]), "allow_insecure") {
			out["skip-cert-verify"] = true
		}
		if alpn := options["alpn"]; alpn != nil {
			out["alpn"] = alpn
		} else if ts := fluxRecord(n["tlsSettings"]); ts != nil && ts["alpn"] != nil {
			out["alpn"] = ts["alpn"]
		}
	case "obfs":
		if nodeObfs != "" {
			out["obfs-key"] = nodeObfs
		}
	}

	var rawList []any
	switch typed := options["channels"].(type) {
	case []any:
		rawList = typed
	case []map[string]any:
		for _, item := range typed {
			rawList = append(rawList, item)
		}
	case []p.Record:
		for _, item := range typed {
			rawList = append(rawList, item)
		}
	}
	if len(rawList) == 0 {
		return
	}

	channels := make([]p.Record, 0, len(rawList))
	for idx, rawItem := range rawList {
		ch := fluxRecord(rawItem)
		if ch == nil {
			continue
		}
		if enabledVal, exists := ch["enabled"]; exists && enabledVal == false {
			continue
		}
		id := fluxFirstText(ch, "id")
		if id == "" {
			id = fmt.Sprintf("ch-%d", idx+1)
		}
		transport, httpMode := fluxCleanTransport(
			fluxFirstText(ch, "transport", "network"),
			fluxFirstText(ch, "httpMode", "http_mode"),
		)
		ipVer := strings.ToLower(fluxFirstText(ch, "ipVersion", "ip_version"))
		if ipVer != "ipv4" && ipVer != "ipv6" {
			ipVer = "auto"
		}
		role := strings.ToLower(fluxFirstText(ch, "role"))
		if role != "backup" {
			role = "primary"
		}
		weight := max(1, min(100, fluxInt(ch, 100, "weight")))

		entry := p.Record{
			"id":         id,
			"enabled":    true,
			"ip-version": ipVer,
			"transport":  transport,
			"role":       role,
			"weight":     weight,
		}
		if host := fluxFirstText(ch, "connectHost", "connect_host", "server"); host != "" {
			entry["server"] = host
		}
		ports := fluxPortString(ch["connectPorts"])
		if ports == "" {
			ports = fluxPortString(ch["connect_ports"])
		}
		if ports == "" {
			ports = fluxPortString(ch["ports"])
		}
		if ports != "" && ports != "0" {
			entry["ports"] = ports
		}

		if transport == "http" {
			entry["http-mode"] = httpMode
			if h := fluxFirstText(ch, "httpHost", "http_host"); h != "" {
				entry["http-host"] = h
			}
			pth := fluxFirstText(ch, "httpPath", "http_path", "path")
			if pth == "" {
				pth = "/"
			} else if !strings.HasPrefix(pth, "/") {
				pth = "/" + pth
			}
			entry["http-path"] = pth
			rawHdr := ch["httpHeaders"]
			if rawHdr == nil {
				rawHdr = ch["http_headers"]
			}
			if hdrs := fluxHeaders(rawHdr); len(hdrs) > 0 {
				entry["http-headers"] = hdrs
			}
		}

		chSec := fluxCleanSecurity(fluxFirstText(ch, "security"))
		if chSec != "" {
			entry["security"] = chSec
		}
		effSec := chSec
		if effSec == "" {
			effSec = defaultSec
		}

		nested := fluxRecord(ch["reality"])
		if nested == nil {
			nested = fluxRecord(ch["realityOpts"])
		}
		if nested == nil {
			nested = fluxRecord(ch["reality_opts"])
		}
		chSNI := fluxFirstText(ch, "sni", "servername", "serverName", "server_name")
		if chSNI == "" && nested != nil {
			chSNI = fluxFirstText(nested, "serverName", "server_name")
		}
		chFP := fluxFirstText(ch, "fingerprint", "clientFingerprint", "client_fingerprint")

		switch effSec {
		case "reality":
			if chSNI != "" {
				entry["sni"] = chSNI
			}
			if chFP != "" && chFP != "inherit" {
				entry["client-fingerprint"] = chFP
			}
			chPub := fluxFirstText(ch, "publicKey", "public_key")
			if chPub == "" && nested != nil {
				chPub = fluxFirstText(nested, "publicKey", "public_key")
			}
			chSID := fluxFirstText(ch, "shortId", "short_id")
			if chSID == "" && nested != nil {
				chSID = fluxFirstText(nested, "shortId", "short_id")
			}
			if chPub != "" || chSID != "" {
				effPub := chPub
				if effPub == "" {
					effPub = nodePub
				}
				effSID := chSID
				if effSID == "" {
					effSID = nodeSID
				}
				if effPub != "" {
					entry["reality-opts"] = p.Record{"public-key": effPub, "short-id": effSID}
				}
			}
		case "tls":
			if chSNI != "" {
				entry["sni"] = chSNI
			}
			if chFP != "" && chFP != "inherit" {
				entry["client-fingerprint"] = chFP
			}
			if p.Bool(ch, "insecure") || p.Bool(ch, "allow_insecure") || p.Bool(ch, "skip-cert-verify") {
				entry["skip-cert-verify"] = true
			}
			if alpn := ch["alpn"]; alpn != nil {
				entry["alpn"] = alpn
			}
		case "obfs":
			if chObfs := fluxFirstText(ch, "obfsKey", "obfs_key"); chObfs != "" {
				entry["obfs-key"] = chObfs
			}
		}
		channels = append(channels, entry)
	}
	if len(channels) > 0 {
		out["channels"] = channels
	}
}

package subscriptions

import (
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/url"
)

func hysteriaObfs(options p.Record) (string, string) {
	if obfs := object(options, "obfs"); obfs != nil {
		if p.Bool(obfs, "open") || p.Bool(obfs, "enabled") {
			return p.Text(obfs, "type"), p.Text(obfs, "password")
		}
		return "", ""
	}
	return p.Text(options, "obfs"), p.Text(options, "obfs_password")
}

func clashProtocolOptions(n, out p.Record) {
	options := object(n, "protocolOptions")
	switch p.Text(n, "protocol") {
	case "hysteria2":
		if kind, password := hysteriaObfs(options); kind != "" {
			out["obfs"], out["obfs-password"] = kind, password
		}
		bandwidth := object(options, "bandwidth")
		if value := bandwidth["up"]; value != nil {
			out["up"] = value
		}
		if value := bandwidth["down"]; value != nil {
			out["down"] = value
		}
	case "tuic":
		for source, target := range map[string]string{"congestion_control": "congestion-controller", "udp_relay_mode": "udp-relay-mode", "zero_rtt_handshake": "reduce-rtt"} {
			if value := options[source]; value != nil {
				out[target] = value
			}
		}
	}
}

func singboxProtocolOptions(n, out p.Record) {
	options := object(n, "protocolOptions")
	switch p.Text(n, "protocol") {
	case "hysteria2":
		if kind, password := hysteriaObfs(options); kind != "" {
			out["obfs"] = p.Record{"type": kind, "password": password}
		}
		bandwidth := object(options, "bandwidth")
		for source, target := range map[string]string{"up": "up_mbps", "down": "down_mbps"} {
			if value := bandwidth[source]; value != nil {
				out[target] = value
			} else if value := options[target]; value != nil {
				out[target] = value
			}
		}
	case "tuic":
		for _, key := range []string{"congestion_control", "udp_relay_mode", "zero_rtt_handshake"} {
			if value := options[key]; value != nil {
				out[key] = value
			}
		}
	}
}

func uriProtocolOptions(n p.Record, q url.Values) {
	options := object(n, "protocolOptions")
	if flow := p.Text(options, "flow"); flow != "" {
		q.Set("flow", flow)
	}
	if p.Text(n, "protocol") == "vless" {
		q.Set("encryption", "none")
	}
	if p.Text(n, "protocol") == "hysteria2" {
		if kind, password := hysteriaObfs(options); kind != "" {
			q.Set("obfs", kind)
			q.Set("obfs-password", password)
		}
	}
	if p.Text(n, "protocol") == "tuic" {
		for _, key := range []string{"congestion_control", "udp_relay_mode"} {
			if value := p.Text(options, key); value != "" {
				q.Set(key, value)
			}
		}
	}
}

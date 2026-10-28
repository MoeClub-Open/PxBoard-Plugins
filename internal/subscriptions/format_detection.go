package subscriptions

import (
	"net/http"
	"strings"
)

// Explicit format wins; otherwise use the client's identity and default to Mihomo.
func subscriptionFormat(r *http.Request) string {
	if format := strings.ToLower(r.URL.Query().Get("format")); format != "" {
		switch format {
		case "mihomo", "meta", "clashmeta", "clash-meta":
			return "clash"
		case "singbox":
			return "sing-box"
		case "raw", "base64":
			return "uri"
		default:
			return format
		}
	}
	client := strings.ToLower(r.URL.Query().Get("flag"))
	if client == "uri" {
		return "uri"
	}
	if client == "" {
		client = strings.ToLower(r.UserAgent())
	}
	for _, name := range []string{"sing-box", "singbox", "hiddify", "sfm"} {
		if strings.Contains(client, name) {
			return "sing-box"
		}
	}
	for _, name := range []string{"v2rayn", "v2rayng", "shadowrocket"} {
		if strings.Contains(client, name) {
			return "uri"
		}
	}
	return "clash"
}

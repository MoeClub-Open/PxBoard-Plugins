package common

import (
	"crypto/sha3"
	"encoding/base64"
	"encoding/hex"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strings"
)

func CanUse(account, node p.Record) bool {
	level := p.Number(account, "accessLevel")
	if expires := p.Text(account, "levelExpiresAt"); expires != "" && expires <= Now() {
		level = 0
	}
	if level < p.Number(node, "minimumAccessLevel") {
		return false
	}
	if allowed, ok := node["allowedAccountIds"].([]any); ok {
		found := false
		for _, id := range allowed {
			if id == account["numericId"] {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if p.Bool(account, "allNodeGroups") {
		return true
	}
	for _, a := range strings.Split(p.Text(account, "groups"), ",") {
		for _, b := range strings.Split(p.Text(node, "groups"), ",") {
			if a != "" && strings.TrimSpace(a) == strings.TrimSpace(b) {
				return true
			}
		}
	}
	return false
}
func PasswordFor(account, node p.Record) string {
	uuid := p.Text(account, "credential")
	password := p.Text(account, "proxyPassword")
	if password == "" {
		password = uuid
	}
	mode := p.Text(node, "passwordMode")
	if mode == "uuid" {
		return uuid
	}
	options, _ := node["protocolOptions"].(map[string]any)
	cipher := p.Text(options, "cipher")
	if cipher == "" {
		cipher = p.Text(options, "method")
	}
	length := 32
	if strings.Contains(cipher, "aes-128") {
		length = 16
	}
	if mode == "sha3-hex-prefix-base64" {
		sum := sha3.Sum256([]byte(password))
		raw := hex.EncodeToString(sum[:])
		return base64.StdEncoding.EncodeToString([]byte(raw[:length]))
	}
	if strings.HasPrefix(cipher, "2022-") {
		if len(uuid) < length {
			return ""
		}
		return base64.StdEncoding.EncodeToString([]byte(uuid[:length]))
	}
	return password
}

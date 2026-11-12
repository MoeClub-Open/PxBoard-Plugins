package subscriptions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) clientProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.Member(w, r) {
		return
	}
	var req struct {
		KnownRevision string   `json:"knownRevision"`
		Protocols     []string `json:"protocols"`
	}
	if err := p.Decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
		p.Fail(w, 400, "请求格式无效")
		return
	}
	subject := p.Subject(r)
	var account p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/accounts/"+url.PathEscape(subject.ID), nil, &account)) {
		return
	}

	accountSummary, blockReason := summarizeClientAccount(account, time.Now())

	links, err := a.userLinksSummary(subject.ID, subject.Email)
	if c.Err(w, err) {
		return
	}
	if blockReason == "" && len(account) > 0 && len(links) == 0 {
		defaultLink := p.Record{
			"id":            "default-" + subject.ID,
			"userId":        subject.ID,
			"name":          "默认订阅",
			"bindUserAgent": false,
			"token":         p.ID() + p.ID(),
			"createdAt":     c.Now(),
		}
		if c.Err(w, a.C.Put("links", defaultLink)) {
			return
		}
		links, err = a.userLinksSummary(subject.ID, subject.Email)
		if c.Err(w, err) {
			return
		}
	}
	policySettings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	subSettings, err := a.subscriptionSettings()
	if c.Err(w, err) {
		return
	}
	title := a.profileTitle(policySettings)
	homepage := p.Text(policySettings, "homepage")
	if !validHomepage(homepage) {
		homepage = ""
	}
	settingsSummary := p.Record{
		"title":               title,
		"homepage":            homepage,
		"updateIntervalHours": int(p.Number(policySettings, "updateIntervalHours")),
		"hideFromBrowsers":    p.Bool(subSettings, "hideFromBrowsers"),
	}

	clientNodes := []p.Record{}
	routing := emptyAerionRouting()

	if blockReason == "" && len(account) > 0 {
		var nodes []p.Record
		if c.Err(w, a.C.Call("nodes", "GET", "/internal/nodes", nil, &nodes)) {
			return
		}
		capReq := buildAerionCapabilityRequest(r.Host, req.Protocols)
		compatible := []p.Record{}
		for _, n := range nodes {
			parent, hasParent := n["parentAccess"].(map[string]any)
			if !c.CanUse(account, n) || (hasParent && !c.CanUse(account, p.Record(parent))) {
				continue
			}
			if supportedFormat(n, "aerion", capReq) {
				compatible = append(compatible, n)
			}
		}
		compatible = sortedNodes(compatible)
		if len(compatible) > 0 {
			generated := make([]map[string]any, 0, len(compatible))
			clientNodes = make([]p.Record, 0, len(compatible))
			email := strings.TrimSpace(p.Text(account, "email"))
			if email == "" {
				email = strings.TrimSpace(subject.Email)
			}
			for _, node := range compatible {
				pwd := subscriptionPassword(account, node)
				uuid := p.Text(account, "credential")
				proxy := aerion(node, pwd, uuid)
				clientNode := aerionClientNode(node, pwd, uuid)
				if p.Text(node, "protocol") == "naive" && email != "" {
					proxy["username"] = email
					clientNode["username"] = email
				}
				generated = append(generated, map[string]any(proxy))
				clientNodes = append(clientNodes, clientNode)
			}
			template, _, loadErr := a.selectedTemplate("clash")
			if c.Err(w, loadErr) {
				return
			}
			hosts, hostErr := a.directHosts(r.Host, homepage)
			if c.Err(w, hostErr) {
				return
			}
			rendered, renderErr := sc.Render("clash", p.Text(template, "content"), generated, title, hosts)
			if c.Err(w, renderErr) {
				return
			}
			_, sanitizedRouting, sanitizeErr := sanitizeAerionYAML(rendered)
			if c.Err(w, sanitizeErr) {
				return
			}
			routing = sanitizedRouting
		}
	}

	revisionPayload, _ := json.Marshal(p.Record{
		"blockReason":         blockReason,
		"entitlementRevision": p.Text(account, "entitlementRevision"),
		"status":              accountSummary["status"],
		"unlimited":           accountSummary["unlimited"],
		"quotaBytes":          accountSummary["quotaBytes"],
		"expiresAt":           accountSummary["expiresAt"],
		"groups":              accountSummary["groups"],
		"speedLimitMbps":      accountSummary["speedLimitMbps"],
		"deviceLimit":         accountSummary["deviceLimit"],
		"nodes":               clientNodes,
		"routing":             routing,
		"settings":            settingsSummary,
	})
	sum := sha256.Sum256(revisionPayload)
	revision := hex.EncodeToString(sum[:])

	if strings.TrimSpace(req.KnownRevision) != "" && strings.TrimSpace(req.KnownRevision) == revision {
		p.JSON(w, p.Record{
			"revision":    revision,
			"unchanged":   true,
			"blockReason": blockReason,
			"account":     accountSummary,
			"links":       links,
			"settings":    settingsSummary,
		})
		return
	}

	p.JSON(w, p.Record{
		"revision":    revision,
		"unchanged":   false,
		"blockReason": blockReason,
		"account":     accountSummary,
		"nodes":       clientNodes,
		"routing":     routing,
		"links":       links,
		"settings":    settingsSummary,
	})
}

func summarizeClientAccount(account p.Record, now time.Time) (p.Record, string) {
	if len(account) == 0 {
		return p.Record{
			"exists":         false,
			"status":         "none",
			"effective":      false,
			"unlimited":      false,
			"usedGiB":        0.0,
			"quotaGiB":       0.0,
			"usedBytes":      int64(0),
			"quotaBytes":     int64(0),
			"expiresAt":      "",
			"expiredAtUnix":  int64(0),
			"cycleStart":     "",
			"speedLimitMbps": 0.0,
			"deviceLimit":    0.0,
			"groups":         "",
			"blockReason":    "no_plan",
		}, "no_plan"
	}
	usedGiB := p.Number(account, "usedGiB")
	quotaGiB := p.Number(account, "quotaGiB")
	usedBytes := int64(math.Round(p.Number(account, "usedBytes")))
	if usedBytes == 0 && usedGiB > 0 {
		usedBytes = int64(math.Round(usedGiB * 1073741824))
	}
	quotaBytes := int64(math.Round(quotaGiB * 1073741824))
	unlimited := p.Bool(account, "unlimited")
	expiresAt := p.Text(account, "expiresAt")
	var expiredAtUnix int64
	expired := false
	if expiresAt != "" {
		if t, err := time.Parse(time.RFC3339, expiresAt); err == nil {
			expiredAtUnix = t.Unix()
			expired = !t.After(now)
		}
	}
	status := p.Text(account, "status")
	blockReason := ""
	switch {
	case status != "active":
		blockReason = "suspended"
	case expired:
		blockReason = "expired"
	case !unlimited && (quotaBytes <= 0 || usedBytes >= quotaBytes):
		blockReason = "traffic_exceeded"
	case !p.Bool(account, "effective"):
		blockReason = "suspended"
	}
	return p.Record{
		"exists":         true,
		"status":         status,
		"effective":      p.Bool(account, "effective") && blockReason == "",
		"unlimited":      unlimited,
		"usedGiB":        usedGiB,
		"quotaGiB":       quotaGiB,
		"usedBytes":      usedBytes,
		"quotaBytes":     quotaBytes,
		"expiresAt":      expiresAt,
		"expiredAtUnix":  expiredAtUnix,
		"cycleStart":     p.Text(account, "cycleStart"),
		"speedLimitMbps": p.Number(account, "speedLimitMbps"),
		"deviceLimit":    p.Number(account, "deviceLimit"),
		"groups":         p.Text(account, "groups"),
		"blockReason":    blockReason,
	}, blockReason
}

func (a *app) userLinksSummary(userID, email string) ([]p.Record, error) {
	rows, err := a.queryLinks([]p.Record{{"field": "userId", "value": userID}})
	if err != nil {
		return nil, err
	}
	policy, err := a.verificationPolicy()
	if err != nil {
		return nil, err
	}
	customBase, err := a.customSubscriptionBase()
	if err != nil {
		return nil, err
	}
	out := make([]p.Record, 0, len(rows))
	for _, row := range rows {
		enrichLinkRecord(row, policy, customBase, email)
		out = append(out, row)
	}
	return out, nil
}

func buildAerionCapabilityRequest(host string, protocols []string) *http.Request {
	target := "/_rpc/client.profile?format=aerion"
	if len(protocols) > 0 {
		cleaned := make([]string, 0, len(protocols))
		for _, item := range protocols {
			if t := strings.TrimSpace(item); t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) > 0 {
			target += "&types=" + url.QueryEscape(strings.Join(cleaned, ","))
		}
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("User-Agent", "XBClient/0.5 Aerion")
	if host != "" {
		req.Host = host
	}
	return req
}

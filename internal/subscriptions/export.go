package subscriptions

import (
	"encoding/base64"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func (a *app) export(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	settings, e := a.subscriptionSettings()
	if c.Err(w, e) {
		return
	}
	if p.Bool(settings, "hideFromBrowsers") && matchesBrowser(r, p.Text(settings, "browserAgents")) {
		disguise(w)
		return
	}
	rows, e := a.queryLinks([]p.Record{{"field": "token", "value": r.PathValue("token")}})
	if c.Err(w, e) {
		return
	}
	if len(rows) == 0 {
		p.Fail(w, 404, "订阅链接不存在或已重置")
		return
	}
	link := rows[0]
	client, major, ok := a.checkClient(w, r, link, p.Text(settings, "browserAgents"))
	if !ok {
		return
	}
	policy, e := a.verificationPolicy()
	if c.Err(w, e) {
		return
	}
	if p.Bool(policy, "enabled") && !verified(link, policy) {
		p.Fail(w, 403, "请先登录面板并完成订阅验证")
		return
	}
	a.exportAccount(w, r, link, client, major)
}

func (a *app) configuration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if c.Member(w, r) {
		a.exportAccount(w, r, p.Record{"userId": p.Subject(r).ID}, "", "")
	}
}

func (a *app) exportAccount(w http.ResponseWriter, r *http.Request, link p.Record, client, major string) {
	var clients, nodes []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients?userId="+url.QueryEscape(p.Text(link, "userId")), nil, &clients)) {
		return
	}
	var account p.Record
	for _, v := range clients {
		if v["userId"] == link["userId"] {
			account = v
		}
	}
	if account == nil {
		p.Fail(w, 403, "账户已到期、暂停或流量不足")
		return
	}
	if c.Err(w, a.C.Call("nodes", "GET", "/internal/nodes", nil, &nodes)) {
		return
	}
	allowed := []p.Record{}
	for _, n := range nodes {
		parent, hasParent := n["parentAccess"].(map[string]any)
		if c.CanUse(account, n) && (!hasParent || c.CanUse(account, p.Record(parent))) {
			allowed = append(allowed, n)
		}
	}

	info := fmt.Sprintf("upload=0; download=%.0f; total=%.0f", p.Number(account, "usedGiB")*1073741824, p.Number(account, "quotaGiB")*1073741824)
	if expires, e := time.Parse(time.RFC3339, p.Text(account, "expiresAt")); e == nil {
		info += fmt.Sprintf("; expire=%d", expires.Unix())
	}
	w.Header().Set("Subscription-Userinfo", info)
	format := subscriptionFormat(r)
	native := r.URL.Query().Get("format") == "aerion" && r.URL.Path == "/configuration"
	if native {
		format = "clash"
	}
	if !c.Allowed(format, "clash", "sing-box", "uri") {
		p.Fail(w, 400, "支持 clash、sing-box、uri 格式")
		return
	}
	compatible := []p.Record{}
	skipped := map[string]bool{}
	for _, n := range allowed {
		protocol := p.Text(n, "protocol")
		if supportedFormat(n, format) {
			compatible = append(compatible, n)
		} else {
			skipped[protocol] = true
		}
	}
	if len(skipped) > 0 {
		protocols := []string{}
		for protocol := range skipped {
			protocols = append(protocols, protocol)
		}
		sort.Strings(protocols)
		w.Header().Set("X-PxBoard-Skipped-Protocols", strings.Join(protocols, ","))
		if len(compatible) == 0 {
			message := "当前格式没有可用节点（协议或传输不支持：" + strings.Join(protocols, "、") + "）"
			if skipped["mieru"] {
				message += "；Mieru 请使用 Mihomo 订阅或支持 mierus 链接的客户端"
			}
			p.Fail(w, 400, message)
			return
		}
	}
	allowed = sortedNodes(compatible)
	if len(allowed) == 0 {
		p.Fail(w, 403, "当前账户没有可用节点，请联系管理员")
		return
	}
	if native {
		rows := []p.Record{}
		for _, node := range allowed {
			proxy := clash(node, subscriptionPassword(account, node), p.Text(account, "credential"))
			proxy["id"], proxy["host"] = node["id"], proxy["server"]
			proxy["insecure"] = p.Bool(proxy, "skip-cert-verify")
			rows = append(rows, proxy)
		}
		p.JSON(w, p.Record{"nodes": rows})
		return
	}
	policySettings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	title := a.profileTitle(policySettings)
	var data []byte
	contentType := "text/plain; charset=utf-8"
	if format == "uri" {
		lines := []string{}
		for _, node := range allowed {
			lines = append(lines, uri(node, subscriptionPassword(account, node), p.Text(account, "credential")))
		}
		data = []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n"))))
	} else {
		template, _, loadErr := a.selectedTemplate(format)
		if c.Err(w, loadErr) {
			return
		}
		generated := []map[string]any{}
		for _, node := range allowed {
			var proxy p.Record
			if format == "clash" {
				proxy = clash(node, subscriptionPassword(account, node), p.Text(account, "credential"))
			} else {
				proxy = singbox(node, subscriptionPassword(account, node), p.Text(account, "credential"))
			}
			generated = append(generated, map[string]any(proxy))
		}
		hosts, hostErr := a.directHosts(r.Host, p.Text(policySettings, "homepage"))
		if c.Err(w, hostErr) {
			return
		}
		data, err = sc.Render(format, p.Text(template, "content"), generated, title, hosts)
		if format == "clash" {
			contentType = "text/yaml; charset=utf-8"
		} else {
			contentType = "application/json; charset=utf-8"
		}
	}

	if err != nil {
		p.Fail(w, 500, "订阅生成失败")
		return
	}
	if !a.bindClient(w, link, client, major) {
		return
	}
	w.Header().Set("profile-update-interval", fmt.Sprintf("%.0f", p.Number(policySettings, "updateIntervalHours")))
	w.Header().Set("profile-title", "base64:"+base64.StdEncoding.EncodeToString([]byte(title)))
	if homepage := p.Text(policySettings, "homepage"); homepage != "" && validHomepage(homepage) {
		w.Header().Set("profile-web-page-url", homepage)
	}
	extension := map[string]string{"clash": "yaml", "sing-box": "json", "uri": "txt"}[format]
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(title+"."+extension))
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

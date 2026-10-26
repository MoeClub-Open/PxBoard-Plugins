package nodes

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

func (a *app) machineNodes(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	rows, e := a.C.List("nodes")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, n := range rows {
		if n["machineId"] == m["number"] && p.Bool(n, "enabled") && p.Text(n, "engine") == "noders" {
			out = append(out, p.Record{"id": n["wireId"], "protocol": n["protocol"], "name": n["name"]})
		}
	}
	m["lastSeen"] = c.Now()
	if c.Err(w, a.C.Put("machines", m)) {
		return
	}
	p.JSON(w, p.Record{"nodes": out, "pullIntervalSeconds": 30, "pushIntervalSeconds": 30})
}
func (a *app) machineConfig(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	n, ok := a.machineNode(w, r, m)
	if !ok {
		return
	}
	port := p.Number(n, "serverPort")
	if port == 0 {
		port = p.Number(n, "port")
	}
	options, _ := n["protocolOptions"].(map[string]any)
	if options == nil {
		options = map[string]any{}
	}
	mode := "none"
	if p.Bool(n, "tls") {
		mode = "tls"
	}
	if p.Text(n, "tlsMode") == "reality" {
		mode = "reality"
	}
	security := p.Record{"mode": mode, "serverName": n["serverName"], "tlsSettings": n["tlsSettings"], "reality": n["realitySettings"], "certificate": n["certConfig"]}
	if security["certificate"] == nil && mode == "tls" {
		security["certificate"] = p.Record{"mode": "file", "cert_path": n["certPath"], "key_path": n["keyPath"]}
	}
	transport := p.Text(n, "transport")
	if transport == "" {
		transport = "tcp"
	}
	p.JSON(w, p.Record{"id": n["wireId"], "protocol": n["protocol"], "host": n["host"], "listen": p.Record{"host": "0.0.0.0", "port": port}, "transport": p.Record{"type": transport, "settings": n["transportSettings"]}, "security": security, "protocolOptions": options})
}
func (a *app) nodeClients(n p.Record) ([]p.Record, error) {
	var all []p.Record
	if e := a.C.Call("access", "GET", "/internal/clients", nil, &all); e != nil {
		return nil, e
	}
	out := []p.Record{}
	for _, user := range all {
		if c.CanUse(user, n) {
			out = append(out, user)
		}
	}
	return out, nil
}
func (a *app) machineUsers(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	n, ok := a.machineNode(w, r, m)
	if !ok {
		return
	}
	users, e := a.nodeClients(n)
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, u := range users {
		password := c.PasswordFor(u, n)
		out = append(out, p.Record{"accountId": u["numericId"], "userId": u["userId"], "email": u["email"], "credential": p.Record{"uuid": u["credential"], "password": password}, "speedLimitMbps": p.Number(u, "speedLimitMbps"), "deviceLimit": p.Number(u, "deviceLimit")})
	}
	p.JSON(w, out)
}
func (a *app) machineTraffic(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	n, ok := a.machineNode(w, r, m)
	if !ok {
		return
	}
	var b struct {
		BatchID  string `json:"batchId"`
		Accounts []struct {
			AccountID int64  `json:"accountId"`
			Upload    uint64 `json:"uploadBytes"`
			Download  uint64 `json:"downloadBytes"`
		} `json:"accounts"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if b.BatchID == "" {
		p.Fail(w, 400, "缺少流量批次标识")
		return
	}
	id := fmt.Sprintf("machine-%.0f-node-%.0f-%s", p.Number(m, "number"), p.Number(n, "wireId"), b.BatchID)
	seen, e := a.C.List("reports")
	if c.Err(w, e) {
		return
	}
	for _, row := range seen {
		if row["id"] == id {
			p.JSON(w, p.Record{"accepted": true, "duplicate": true})
			return
		}
	}
	var users []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/accounts", nil, &users)) {
		return
	}
	byID := map[int64]p.Record{}
	for _, u := range users {
		byID[int64(p.Number(u, "numericId"))] = u
	}
	usage := []p.Record{}
	for _, u := range b.Accounts {
		user := byID[u.AccountID]
		if user == nil {
			p.Fail(w, 409, fmt.Sprintf("流量账户 %d 不存在，批次尚未确认", u.AccountID))
			return
		}
		usage = append(usage, p.Record{"userId": user["userId"], "bytes": u.Upload + u.Download})
	}
	if len(usage) > 0 && c.Err(w, a.C.Emit("traffic.reported", p.Record{"nodeId": n["id"], "reportId": id, "rate": n["trafficRate"], "usage": usage})) {
		return
	}
	if c.Err(w, a.C.Put("reports", p.Record{"id": id, "createdAt": c.Now()})) {
		return
	}
	n["lastSeen"] = c.Now()
	if c.Err(w, a.C.Put("nodes", n)) {
		return
	}
	p.JSON(w, p.Record{"accepted": true})
}
func (a *app) machineTelemetry(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	m["lastSeen"] = c.Now()
	if r.PathValue("node") == "" {
		m["status"] = b
		m["statusUpdatedAt"] = c.Now()
		if !c.Err(w, a.C.Put("machines", m)) {
			p.JSON(w, true)
		}
		return
	}
	n, ok := a.machineNode(w, r, m)
	if !ok {
		return
	}
	n["lastSeen"] = c.Now()
	if accounts, ok := b["accounts"]; ok {
		n["onlineAccounts"] = accounts
		n["onlineUpdatedAt"] = c.Now()
	} else {
		n["status"] = b
		n["statusUpdatedAt"] = c.Now()
	}
	if !c.Err(w, a.C.Put("nodes", n)) {
		p.JSON(w, true)
	}
}

package nodes

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const lastSeenWriteThrottle = 45 * time.Second

func (a *app) serverRPC() {
	a.RPC("server.bootstrap", a.serverBootstrap)
	a.RPC("server.pull", a.serverPull)
	a.RPC("server.traffic", a.serverTraffic)
	a.RPC("server.telemetry", a.serverTelemetry)
	a.RPC("server.ackConfig", a.serverAckConfig)
}

func (a *app) requireMachine(w http.ResponseWriter, r *http.Request, requestedID int64) (p.Record, bool) {
	actor, ok := p.Machine(r)
	if !ok {
		p.Fail(w, 401, "服务器凭据无效或服务器已停用")
		return nil, false
	}
	if requestedID > 0 && requestedID != actor.MachineID {
		p.Fail(w, 403, "服务器编号与凭据不匹配")
		return nil, false
	}
	m, err := a.Find("machines", actor.ID)
	if err != nil || !p.Bool(m, "enabled") {
		p.Fail(w, 401, "服务器凭据无效或服务器已停用")
		return nil, false
	}
	return m, true
}

func (a *app) touchMachine(machineDocID string, force bool) error {
	a.machineMu.Lock()
	defer a.machineMu.Unlock()
	m, err := a.Find("machines", machineDocID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if !force {
		if prev, parseErr := time.Parse(time.RFC3339, p.Text(m, "lastSeen")); parseErr == nil && now.Sub(prev) < lastSeenWriteThrottle {
			return nil
		}
	}
	m["lastSeen"] = now.Format(time.RFC3339)
	return a.C.Put("machines", m)
}

func buildMachineNodeConfig(n p.Record) p.Record {
	port := p.Number(n, "serverPort")
	if port == 0 {
		port = p.Number(n, "port")
	}
	transport := p.Text(n, "transport")
	if transport == "" {
		transport = "tcp"
	}
	if p.Text(n, "protocol") == "flux" {
		sec, fluxOptions := fluxMachineSecurityAndOptions(n)
		return p.Record{
			"id":              n["wireId"],
			"protocol":        n["protocol"],
			"host":            n["host"],
			"listen":          p.Record{"host": "0.0.0.0", "port": port},
			"transport":       p.Record{"type": transport, "settings": n["transportSettings"]},
			"security":        sec,
			"protocolOptions": fluxOptions,
		}
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
	return p.Record{
		"id":              n["wireId"],
		"protocol":        n["protocol"],
		"host":            n["host"],
		"listen":          p.Record{"host": "0.0.0.0", "port": port},
		"transport":       p.Record{"type": transport, "settings": n["transportSettings"]},
		"security":        security,
		"protocolOptions": options,
	}
}

func buildNodeUsers(allClients []p.Record, n p.Record) []p.Record {
	out := []p.Record{}
	for _, u := range allClients {
		if !c.CanUse(u, n) {
			continue
		}
		password := c.PasswordFor(u, n)
		out = append(out, p.Record{
			"accountId":      u["numericId"],
			"userId":         u["userId"],
			"email":          u["email"],
			"credential":     p.Record{"uuid": u["credential"], "password": password},
			"speedLimitMbps": p.Number(u, "speedLimitMbps"),
			"deviceLimit":    p.Number(u, "deviceLimit"),
		})
	}
	slices.SortFunc(out, func(a, b p.Record) int {
		na, nb := p.Number(a, "accountId"), p.Number(b, "accountId")
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		return 0
	})
	return out
}

func (a *app) serverBootstrap(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID int64 `json:"machineId"`
	}
	if err := p.Decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
		p.Fail(w, 400, "请求格式无效")
		return
	}
	m, ok := a.requireMachine(w, r, req.MachineID)
	if !ok {
		return
	}
	_ = a.touchMachine(p.Text(m, "id"), true)
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, p.Record{
		"apiVersion":          "1.0",
		"rpcVersion":          "1.0",
		"machineId":           int64(p.Number(m, "number")),
		"name":                p.Text(m, "name"),
		"transport":           "rpc",
		"pullIntervalSeconds": 30,
		"pushIntervalSeconds": 30,
		"capabilities": []string{
			"multiNode",
			"batchSync",
			"revisionCache",
			"trafficOutbox",
			"onlineAccounts",
			"managedConfiguration",
		},
	})
}

func (a *app) serverPull(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID     int64  `json:"machineId"`
		KnownRevision string `json:"knownRevision"`
	}
	if err := p.Decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
		p.Fail(w, 400, "请求格式无效")
		return
	}
	m, ok := a.requireMachine(w, r, req.MachineID)
	if !ok {
		return
	}
	rows, err := a.C.List("nodes")
	if c.Err(w, err) {
		return
	}
	machineNum := int64(p.Number(m, "number"))
	machineNodes := []p.Record{}
	for _, n := range rows {
		if int64(p.Number(n, "machineId")) == machineNum && p.Bool(n, "enabled") {
			machineNodes = append(machineNodes, n)
		}
	}
	slices.SortFunc(machineNodes, func(a, b p.Record) int {
		wa, wb := p.Number(a, "wireId"), p.Number(b, "wireId")
		if wa < wb {
			return -1
		}
		if wa > wb {
			return 1
		}
		return 0
	})

	var allClients []p.Record
	if len(machineNodes) > 0 {
		if c.Err(w, a.C.Call("access", "GET", "/internal/clients", nil, &allClients)) {
			return
		}
	}

	managedCfg, err := a.pendingMachineConfiguration(p.Text(m, "id"))
	if c.Err(w, err) {
		return
	}

	snapshots := make([]p.Record, 0, len(machineNodes))
	for _, n := range machineNodes {
		snapshots = append(snapshots, p.Record{
			"id":       int64(p.Number(n, "wireId")),
			"protocol": p.Text(n, "protocol"),
			"name":     p.Text(n, "name"),
			"config":   buildMachineNodeConfig(n),
			"users":    buildNodeUsers(allClients, n),
		})
	}

	revRaw, _ := json.Marshal(p.Record{
		"nodes":                snapshots,
		"managedConfiguration": managedCfg,
	})
	sum := sha256.Sum256(revRaw)
	revision := hex.EncodeToString(sum[:])

	_ = a.touchMachine(p.Text(m, "id"), false)
	w.Header().Set("Cache-Control", "no-store")

	if strings.TrimSpace(req.KnownRevision) != "" && strings.TrimSpace(req.KnownRevision) == revision {
		p.JSON(w, p.Record{
			"revision":             revision,
			"unchanged":            true,
			"pullIntervalSeconds":  30,
			"pushIntervalSeconds":  30,
			"managedConfiguration": managedCfg,
		})
		return
	}

	p.JSON(w, p.Record{
		"revision":             revision,
		"unchanged":            false,
		"pullIntervalSeconds":  30,
		"pushIntervalSeconds":  30,
		"managedConfiguration": managedCfg,
		"nodes":                snapshots,
	})
}

type trafficAccountItem struct {
	AccountID int64  `json:"accountId"`
	Upload    uint64 `json:"uploadBytes"`
	Download  uint64 `json:"downloadBytes"`
}

type trafficBatchItem struct {
	NodeID   int64                `json:"nodeId"`
	BatchID  string               `json:"batchId"`
	Accounts []trafficAccountItem `json:"accounts"`
}

func (a *app) serverTraffic(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID int64                `json:"machineId"`
		NodeID    int64                `json:"nodeId"`
		BatchID   string               `json:"batchId"`
		Accounts  []trafficAccountItem `json:"accounts"`
		Batches   []trafficBatchItem   `json:"batches"`
	}
	if c.Err(w, p.Decode(r, &req)) {
		return
	}
	m, ok := a.requireMachine(w, r, req.MachineID)
	if !ok {
		return
	}
	batches := req.Batches
	if len(batches) == 0 && req.BatchID != "" {
		batches = []trafficBatchItem{{
			NodeID:   req.NodeID,
			BatchID:  req.BatchID,
			Accounts: req.Accounts,
		}}
	}
	if len(batches) == 0 {
		p.Fail(w, 400, "缺少流量批次标识")
		return
	}

	nodeRows, err := a.C.List("nodes")
	if c.Err(w, err) {
		return
	}
	machineNum := int64(p.Number(m, "number"))
	nodeByWire := map[int64]p.Record{}
	for _, n := range nodeRows {
		if int64(p.Number(n, "machineId")) == machineNum && p.Bool(n, "enabled") {
			nodeByWire[int64(p.Number(n, "wireId"))] = n
		}
	}

	var users []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/accounts", nil, &users)) {
		return
	}
	userByNumeric := map[int64]p.Record{}
	for _, u := range users {
		userByNumeric[int64(p.Number(u, "numericId"))] = u
	}

	processed := 0
	for _, batch := range batches {
		if strings.TrimSpace(batch.BatchID) == "" {
			p.Fail(w, 400, "缺少流量批次标识")
			return
		}
		n := nodeByWire[batch.NodeID]
		if n == nil {
			p.Fail(w, 404, "节点不属于此服务器或已停用")
			return
		}
		reportID := fmt.Sprintf("machine-%d-node-%d-%s", machineNum, batch.NodeID, batch.BatchID)
		seen, err := a.hasReport(reportID)
		if c.Err(w, err) {
			return
		}
		if seen {
			continue
		}
		usage := make([]p.Record, 0, len(batch.Accounts))
		for _, item := range batch.Accounts {
			user := userByNumeric[item.AccountID]
			if user == nil {
				p.Fail(w, 409, fmt.Sprintf("流量账户 %d 不存在，批次尚未确认", item.AccountID))
				return
			}
			usage = append(usage, p.Record{"userId": user["userId"], "bytes": item.Upload + item.Download})
		}
		if len(usage) > 0 && c.Err(w, a.C.Emit("traffic.reported", p.Record{"nodeId": n["id"], "reportId": reportID, "rate": n["trafficRate"], "usage": usage})) {
			return
		}
		if c.Err(w, a.C.Put("reports", p.Record{"id": reportID, "machineId": machineNum, "nodeId": batch.NodeID, "batchId": batch.BatchID, "accounts": batch.Accounts, "trafficRate": n["trafficRate"], "createdAt": c.Now()})) {
			return
		}
		processed++
	}
	_ = a.touchMachine(p.Text(m, "id"), false)
	p.JSON(w, p.Record{"accepted": true, "processed": processed})
}

func (a *app) serverTelemetry(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID int64      `json:"machineId"`
		Status    p.Record   `json:"status"`
		NodeID    int64      `json:"nodeId"`
		Accounts  []p.Record `json:"accounts"`
		Nodes     []struct {
			NodeID   int64      `json:"nodeId"`
			Accounts []p.Record `json:"accounts"`
		} `json:"nodes"`
	}
	if c.Err(w, p.Decode(r, &req)) {
		return
	}
	m, ok := a.requireMachine(w, r, req.MachineID)
	if !ok {
		return
	}
	now := c.Now()
	a.machineMu.Lock()
	freshM, err := a.Find("machines", p.Text(m, "id"))
	if err == nil {
		freshM["lastSeen"] = now
		if len(req.Status) > 0 {
			freshM["status"] = req.Status
			freshM["statusUpdatedAt"] = now
		}
		err = a.C.Put("machines", freshM)
	}
	a.machineMu.Unlock()
	if c.Err(w, err) {
		return
	}

	nodeUpdates := req.Nodes
	if req.NodeID > 0 {
		nodeUpdates = append(nodeUpdates, struct {
			NodeID   int64      `json:"nodeId"`
			Accounts []p.Record `json:"accounts"`
		}{NodeID: req.NodeID, Accounts: req.Accounts})
	}
	if len(nodeUpdates) > 0 {
		rows, err := a.C.List("nodes")
		if c.Err(w, err) {
			return
		}
		machineNum := int64(p.Number(m, "number"))
		byWire := map[int64]p.Record{}
		for _, n := range rows {
			if int64(p.Number(n, "machineId")) == machineNum && p.Bool(n, "enabled") {
				byWire[int64(p.Number(n, "wireId"))] = n
			}
		}
		for _, item := range nodeUpdates {
			n := byWire[item.NodeID]
			if n == nil {
				p.Fail(w, 404, "节点不属于此服务器或已停用")
				return
			}
			accts := item.Accounts
			if accts == nil {
				accts = []p.Record{}
			}
			n["onlineAccounts"] = accts
			n["onlineUpdatedAt"] = now
			delete(n, "status")
			delete(n, "statusUpdatedAt")
			delete(n, "lastSeen")
			if c.Err(w, a.C.Put("nodes", n)) {
				return
			}
		}
	}
	p.JSON(w, true)
}

func (a *app) serverAckConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MachineID int64   `json:"machineId"`
		Revision  float64 `json:"revision"`
		Status    string  `json:"status"`
		Message   string  `json:"message"`
	}
	if c.Err(w, p.Decode(r, &req)) {
		return
	}
	m, ok := a.requireMachine(w, r, req.MachineID)
	if !ok {
		return
	}
	a.ackMachineConfiguration(w, m, req.Revision, req.Status, req.Message)
}

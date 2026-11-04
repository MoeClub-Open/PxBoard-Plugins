package nodes

import (
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const (
	heartbeatTimeout = 120 * time.Second
	telemetryTimeout = 180 * time.Second
)

func (a *app) monitoring() {
	a.Mux.HandleFunc("GET /status", a.status)
	a.Mux.HandleFunc("GET /ui/status", func(w http.ResponseWriter, r *http.Request) {
		if !c.Member(w, r) || !a.requireUserStatus(w, r) {
			return
		}
		view, e := a.statusView(r)
		if !c.Err(w, e) {
			p.JSON(w, view)
		}
	})
}

func (a *app) status(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) || !a.requireUserStatus(w, r) {
		return
	}
	nodes, e := a.C.List("nodes")
	if c.Err(w, e) {
		return
	}
	machines, e := a.C.List("machines")
	if c.Err(w, e) {
		return
	}
	admin := p.Admin(r)
	mine := !admin || r.URL.Query().Get("scope") == "mine"
	var fields []string
	if mine {
		config, err := a.config()
		if c.Err(w, err) {
			return
		}
		fields = selectedStatusFields(config)
	}
	accounts := []p.Record{}
	if mine {
		userID := p.Subject(r).ID
		var all []p.Record
		if c.Err(w, a.C.Call("access", "GET", "/internal/clients?userId="+url.QueryEscape(userID), nil, &all)) {
			return
		}
		for _, account := range all {
			if p.Text(account, "userId") == userID {
				accounts = append(accounts, account)
			}
		}
	}
	rows, _ := buildServerStatuses(machines, nodes, accounts, mine, fields, time.Now())
	p.JSON(w, rows)
}

func buildServerStatuses(machines, nodes, accounts []p.Record, mine bool, fields []string, now time.Time) ([]p.Record, int) {
	sorted := make([]p.Record, len(machines))
	copy(sorted, machines)
	sortServers(sorted)

	byID := map[string]p.Record{}
	for _, node := range nodes {
		byID[p.Text(node, "id")] = node
	}
	serviceByServer := map[int64][]p.Record{}
	entriesByServer := map[int64][]p.Record{}
	for _, node := range nodes {
		parent := byID[p.Text(node, "parentId")]
		service := node
		if !p.Bool(node, "enabled") && parent != nil {
			service = parent
		}
		if p.Text(service, "engine") != "noders" {
			continue
		}
		num := int64(p.Number(service, "machineId"))
		entriesByServer[num] = append(entriesByServer[num], node)
		if p.Bool(node, "enabled") {
			serviceByServer[num] = append(serviceByServer[num], node)
		}
	}

	out := make([]p.Record, 0, len(sorted))
	globalAccounts := map[int64]bool{}
	extraUnnamed := 0
	for _, machine := range sorted {
		num := int64(p.Number(machine, "number"))
		if mine {
			if !p.Bool(machine, "enabled") || !serverAccessible(entriesByServer[num], byID, accounts) {
				continue
			}
		}
		row, activeIDs, unnamed := serverStatus(machine, serviceByServer[num], now)
		for id := range activeIDs {
			globalAccounts[id] = true
		}
		extraUnnamed += unnamed
		if mine {
			row = filterUserStatus(row, fields)
		}
		out = append(out, row)
	}
	return out, len(globalAccounts) + extraUnnamed
}

func serverAccessible(entries []p.Record, byID map[string]p.Record, accounts []p.Record) bool {
	for _, node := range entries {
		parent := byID[p.Text(node, "parentId")]
		enabled := p.Bool(node, "enabled") || p.Bool(parent, "enabled")
		if !nodeVisible(node) || !enabled {
			continue
		}
		for _, account := range accounts {
			if c.CanUse(account, node) && (p.Bool(node, "enabled") || parent == nil || c.CanUse(account, parent)) {
				return true
			}
		}
	}
	return false
}

func primaryNodes(nodes []p.Record) []p.Record {
	byID := map[string]bool{}
	for _, node := range nodes {
		byID[p.Text(node, "id")] = true
	}
	primary := make([]p.Record, 0, len(nodes))
	for _, node := range nodes {
		if p.Bool(node, "enabled") || !byID[p.Text(node, "parentId")] {
			primary = append(primary, node)
		}
	}
	return primary
}

func onlineServerCount(machines []p.Record, now time.Time) int {
	online := 0
	for _, machine := range machines {
		state, _, _ := serverHeartbeatState(machine, now)
		if state == "online" {
			online++
		}
	}
	return online
}

func serverHeartbeatState(machine p.Record, now time.Time) (string, string, int) {
	if !p.Bool(machine, "enabled") {
		return "disabled", "已停用", -1
	}
	status, label := "never", "未接入"
	lastSeen := p.Text(machine, "lastSeen")
	seen, e := time.Parse(time.RFC3339, lastSeen)
	age := -1
	if e == nil {
		status, label = "offline", "离线"
		age = max(0, int(now.Sub(seen).Seconds()))
		if now.Sub(seen) < heartbeatTimeout {
			status, label = "online", "在线"
		}
	}
	return status, label, age
}

func roundPercent(value float64) float64 {
	return math.Round(value*10) / 10
}

func serverStatus(machine p.Record, serviceNodes []p.Record, now time.Time) (p.Record, map[int64]bool, int) {
	status, label, age := serverHeartbeatState(machine, now)
	enabled := p.Bool(machine, "enabled")
	nodeCount := 0
	if enabled {
		nodeCount = len(serviceNodes)
	}
	region := strings.TrimSpace(p.Text(machine, "region"))
	if region == "" {
		for _, n := range serviceNodes {
			if r := strings.TrimSpace(p.Text(n, "region")); r != "" {
				region = r
				break
			}
		}
	}
	row := p.Record{
		"id":          machine["id"],
		"number":      machine["number"],
		"name":        machine["name"],
		"status":      status,
		"statusLabel": label,
		"enabled":     enabled,
		"nodeCount":   nodeCount,
		"lastSeen":    p.Text(machine, "lastSeen"),
	}
	if region != "" {
		row["region"] = region
	}
	if age >= 0 {
		row["heartbeatAgeSeconds"] = age
	}
	activeAccounts := map[int64]bool{}
	unnamedAccounts := 0
	if status != "online" {
		return row, activeAccounts, 0
	}
	telemetry, _ := machine["status"].(map[string]any)
	updated := p.Text(machine, "statusUpdatedAt")
	if updatedAt, e := time.Parse(time.RFC3339, updated); telemetry != nil && e == nil && now.Sub(updatedAt) < telemetryTimeout {
		row["loadUpdatedAt"] = updated
		if value, ok := telemetry["cpuPercent"]; ok && value != nil {
			row["cpuPercent"] = roundPercent(p.Number(telemetry, "cpuPercent"))
		}
		for _, spec := range []struct{ source, key string }{{"memory", "memory"}, {"disk", "disk"}, {"swap", "swap"}} {
			stat, _ := telemetry[spec.source].(map[string]any)
			if total := p.Number(stat, "total"); total > 0 {
				used := p.Number(stat, "used")
				row[spec.key+"Percent"] = roundPercent(used / total * 100)
				row[spec.key+"UsedBytes"] = used
				row[spec.key+"TotalBytes"] = total
			}
		}
	}
	hasAliveReport := false
	for _, node := range serviceNodes {
		aliveAt, e := time.Parse(time.RFC3339, p.Text(node, "onlineUpdatedAt"))
		if e != nil || now.Sub(aliveAt) >= telemetryTimeout {
			continue
		}
		hasAliveReport = true
		accounts, _ := node["onlineAccounts"].([]any)
		for _, value := range accounts {
			account, _ := value.(map[string]any)
			ips, _ := account["ips"].([]any)
			if len(ips) > 0 {
				if id := int64(p.Number(account, "accountId")); id > 0 {
					activeAccounts[id] = true
				} else {
					unnamedAccounts++
				}
			}
		}
	}
	if hasAliveReport {
		row["onlineAccounts"] = len(activeAccounts) + unnamedAccounts
	}
	return row, activeAccounts, unnamedAccounts
}

func (a *app) statusView(r *http.Request) (p.View, error) {
	if p.Admin(r) {
		machines, e := a.C.List("machines")
		if e != nil {
			return p.View{}, e
		}
		nodes, e := a.C.List("nodes")
		if e != nil {
			return p.View{}, e
		}
		now := time.Now()
		rows, onlineUsers := buildServerStatuses(machines, nodes, nil, false, nil, now)
		hasRegion := false
		for _, row := range rows {
			if p.Text(row, "region") != "" {
				hasRegion = true
				break
			}
		}
		online := onlineServerCount(machines, now)
		columns := []p.Field{
			c.Field("number", "编号", "number", false),
			c.Field("name", "服务器", "text", false),
		}
		if hasRegion {
			columns = append(columns, c.Field("region", "地区", "text", false))
		}
		columns = append(columns,
			c.Field("status", "状态", "status", false),
			c.Field("nodeCount", "运行节点", "number", false),
			c.Field("onlineAccounts", "在线账户", "number", false),
			c.Field("cpuPercent", "CPU（%）", "number", false),
			c.Field("memoryPercent", "内存（%）", "number", false),
			c.Field("diskPercent", "磁盘（%）", "number", false),
			c.Field("swapPercent", "交换空间（%）", "number", false),
			c.Field("lastSeen", "最近心跳", "datetime", false),
		)
		metrics := []p.Metric{
			{Label: "全部服务器", Value: len(machines)},
			{Label: "在线服务器", Value: online},
			{Label: "离线或停用", Value: len(machines) - online},
			{Label: "在线账户", Value: onlineUsers},
		}
		return p.View{
			Title:     "服务器监控",
			Metrics:   metrics,
			Resources: []p.Resource{{Key: "status", Title: "服务器", Endpoint: "/status", PollIntervalSeconds: 10, Columns: columns, Empty: "暂无服务器"}},
		}, nil
	}
	config, err := a.config()
	if err != nil {
		return p.View{}, err
	}
	columns := []p.Field{c.Field("name", "服务器", "text", false)}
	for _, key := range selectedStatusFields(config) {
		for _, option := range statusOptions {
			if option.Value == key {
				kind := "number"
				switch key {
				case "region":
					kind = "text"
				case "status":
					kind = "status"
				case "lastSeen":
					kind = "datetime"
				}
				columns = append(columns, c.Field(key, option.Label, kind, false))
			}
		}
	}
	return p.View{
		Title:     "服务器状态",
		Resources: []p.Resource{{Key: "status", Title: "服务器", Endpoint: "/status", PollIntervalSeconds: 10, Columns: columns, Empty: "暂无可查看的服务器"}},
	}, nil
}

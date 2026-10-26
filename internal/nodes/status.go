package nodes

import (
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const heartbeatTimeout = 120 * time.Second

func (a *app) monitoring() {
	a.Mux.HandleFunc("GET /status", a.status)
	a.View("status", a.statusView)
}

func (a *app) status(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
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
	accounts := []p.Record{}
	if mine {
		var all []p.Record
		if c.Err(w, a.C.Call("access", "GET", "/internal/clients", nil, &all)) {
			return
		}
		for _, account := range all {
			if p.Text(account, "userId") == p.Subject(r).ID {
				accounts = append(accounts, account)
			}
		}
	}
	out := []p.Record{}
	byID := map[string]p.Record{}
	for _, node := range nodes {
		byID[p.Text(node, "id")] = node
	}
	for _, node := range nodes {
		parent := byID[p.Text(node, "parentId")]
		enabled := p.Bool(node, "enabled") || p.Bool(parent, "enabled")
		telemetryNode := node
		if !p.Bool(node, "enabled") && parent != nil {
			telemetryNode = parent
		}
		if mine {
			allowed := false
			for _, account := range accounts {
				if nodeVisible(node) && enabled && c.CanUse(account, node) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		var machine p.Record
		if p.Text(telemetryNode, "engine") == "noders" {
			for _, candidate := range machines {
				if p.Number(candidate, "number") == p.Number(telemetryNode, "machineId") {
					machine = candidate
					break
				}
			}
		}
		statusNode := telemetryNode
		if !p.Bool(node, "enabled") && parent != nil {
			statusNode = p.Record{}
			for key, value := range parent {
				statusNode[key] = value
			}
			for _, key := range []string{"id", "name", "region", "protocol", "trafficRate"} {
				statusNode[key] = node[key]
			}
		}
		row := nodeStatus(statusNode, machine, admin && !mine)
		if parent != nil && admin && !mine {
			row["parentName"] = parent["name"]
		}
		out = append(out, row)
	}
	p.JSON(w, out)
}

func nodeStatus(node, machine p.Record, details bool) p.Record {
	status, label := "never", "未接入"
	lastSeen := p.Text(node, "lastSeen")
	seen, e := time.Parse(time.RFC3339, lastSeen)
	if e == nil {
		status, label = "offline", "离线"
		if time.Since(seen) < heartbeatTimeout {
			status, label = "online", "在线"
		}
	}
	if status == "online" && p.Text(node, "error") != "" {
		status, label = "error", "运行异常"
	}
	enabled := p.Bool(node, "enabled") && (machine == nil || p.Bool(machine, "enabled"))
	if !enabled {
		status, label = "disabled", "已停用"
	}
	row := p.Record{"id": node["id"], "name": node["name"], "region": node["region"], "protocol": node["protocol"], "status": status, "statusLabel": label, "enabled": enabled, "lastSeen": lastSeen, "trafficRate": node["trafficRate"]}
	if !details {
		return row
	}
	row["engine"] = node["engine"]
	row["error"] = node["error"]
	if e == nil {
		row["heartbeatAgeSeconds"] = max(0, int(time.Since(seen).Seconds()))
	}
	if machine != nil {
		row["machineName"] = machine["name"]
		row["machineLastSeen"] = machine["lastSeen"]
	}
	telemetry, _ := node["status"].(map[string]any)
	updated := p.Text(node, "statusUpdatedAt")
	if telemetry == nil && machine != nil {
		telemetry, _ = machine["status"].(map[string]any)
		updated = p.Text(machine, "statusUpdatedAt")
	}
	if telemetry != nil && updated != "" {
		row["loadUpdatedAt"] = updated
		if value, ok := telemetry["cpuPercent"]; ok {
			row["cpuPercent"] = value
		}
		for _, spec := range []struct{ source, key string }{{"memory", "memory"}, {"disk", "disk"}, {"swap", "swap"}} {
			stat, _ := telemetry[spec.source].(map[string]any)
			if total := p.Number(stat, "total"); total > 0 {
				used := p.Number(stat, "used")
				row[spec.key+"Percent"] = used / total * 100
				row[spec.key+"UsedBytes"] = used
				row[spec.key+"TotalBytes"] = total
			}
		}
	}
	if aliveAt, e := time.Parse(time.RFC3339, p.Text(node, "onlineUpdatedAt")); e == nil && time.Since(aliveAt) < heartbeatTimeout {
		accounts, _ := node["onlineAccounts"].([]any)
		count := 0
		for _, value := range accounts {
			account, _ := value.(map[string]any)
			ips, _ := account["ips"].([]any)
			if len(ips) > 0 {
				count++
			}
		}
		row["onlineAccounts"] = count
	}
	return row
}

func (a *app) statusView(r *http.Request) (p.View, error) {
	columns := []p.Field{c.Field("name", "节点", "text", false), c.Field("region", "地区", "text", false), c.Field("protocol", "协议", "tag", false), c.Field("statusLabel", "状态", "text", false), c.Field("trafficRate", "流量倍率", "number", false), c.Field("lastSeen", "最近心跳", "datetime", false)}
	if p.Admin(r) {
		columns = append(columns, c.Field("machineName", "机器", "text", false), c.Field("parentName", "共用节点", "text", false), c.Field("onlineAccounts", "在线账户", "number", false), c.Field("cpuPercent", "CPU（%）", "number", false), c.Field("memoryPercent", "内存（%）", "number", false), c.Field("diskPercent", "磁盘（%）", "number", false), c.Field("loadUpdatedAt", "负载更新时间", "datetime", false), c.Field("error", "运行错误", "text", false))
	}
	return p.View{Title: "节点状态", Resources: []p.Resource{{Key: "status", Title: "节点", Endpoint: "/status", PollIntervalSeconds: 10, Columns: columns, Empty: "暂无可查看的节点"}}}, nil
}

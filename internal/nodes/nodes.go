package nodes

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"time"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.machineAPI()
	a.monitoring()
	res := resource()
	a.CRUD(res, a.normalize)
	a.View("nodes", a.view)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		if !p.Admin(r) {
			return nil, fmt.Errorf("无权查看节点概况")
		}
		rows, e := a.C.List("nodes")
		online := 0
		for _, row := range rows {
			seen, err := time.Parse(time.RFC3339, p.Text(row, "lastSeen"))
			if err == nil && time.Since(seen) < heartbeatTimeout {
				online++
			}
		}
		return []p.Metric{{Label: "在线节点", Value: online}, {Label: "全部节点", Value: len(rows)}}, e
	})
	a.Mux.HandleFunc("POST /nodes/{id}/rotate", a.rotate)
	a.Mux.HandleFunc("POST /agent/sync", a.sync)
	a.Mux.HandleFunc("GET /internal/nodes", a.internal)
	return a
}
func resource() p.Resource {
	return p.Resource{Key: "nodes", Title: "节点", Endpoint: "/nodes", CreateLabel: "添加节点", Editable: true, Deletable: true, Empty: "添加一台服务器，开始连接节点", Columns: []p.Field{c.Field("name", "节点名称", "text", false), c.Field("region", "地区", "text", false), c.Field("host", "服务器", "text", false), c.Field("protocol", "协议", "tag", false), c.Field("lastSeen", "最近在线", "datetime", false), c.Field("error", "运行错误", "text", false), c.Field("enabled", "启用", "boolean", false)}, Fields: []p.Field{c.Field("name", "节点名称", "text", true), c.Field("region", "地区", "text", false), c.Field("host", "服务器地址", "text", true), c.Field("port", "端口", "number", true), c.Select("engine", "代理内核", "xray|Xray", "sing-box|sing-box", "noders|NodeRS"), c.Select("protocol", "协议", "vless|VLESS", "vmess|VMess", "trojan|Trojan", "shadowsocks|Shadowsocks", "hysteria2|Hysteria 2", "anytls|AnyTLS", "tuic|TUIC", "mieru|Mieru", "sudoku|Sudoku", "naive|Naive"), c.Field("tls", "启用 TLS", "boolean", false), c.Field("serverName", "证书域名", "text", false), c.Field("certPath", "服务器证书路径", "text", false), c.Field("keyPath", "服务器私钥路径", "text", false), c.Field("groups", "节点分组（逗号分隔）", "text", true), c.Field("trafficRate", "流量倍率", "number", true), c.Field("enabled", "启用节点", "boolean", false)}, Actions: []p.Action{{Key: "rotate", Label: "接入令牌", Confirm: "生成新的节点接入令牌？原令牌会立即失效。"}}}
}
func (a *app) normalize(r *http.Request, b, old p.Record) error {
	if old == nil {
		number, e := a.nextNumber("nodes", "wireId")
		if e != nil {
			return e
		}
		b["wireId"] = number
	} else {
		b["wireId"] = old["wireId"]
	}
	c.Defaults(b, old, p.Record{"engine": "xray", "protocol": "vless", "port": 443.0, "trafficRate": 1.0, "enabled": true, "tls": true, "groups": "default", "agentToken": p.ID() + p.ID()})
	if e := c.Validate(b, "name", "host", "engine", "protocol"); e != nil {
		return e
	}
	if !c.Allowed(p.Text(b, "engine"), "xray", "sing-box", "noders") || !c.Allowed(p.Text(b, "protocol"), "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "anytls", "tuic", "mieru", "sudoku", "naive") {
		return fmt.Errorf("内核或协议无效")
	}
	if p.Text(b, "protocol") == "hysteria2" && p.Text(b, "engine") == "xray" {
		return fmt.Errorf("Hysteria 2 请选择 sing-box")
	}
	if p.Text(b, "engine") == "xray" && !c.Allowed(p.Text(b, "protocol"), "vless", "vmess", "trojan", "shadowsocks") {
		return fmt.Errorf("此协议请选择 NodeRS 或支持它的内核")
	}
	if p.Text(b, "engine") == "sing-box" && !c.Allowed(p.Text(b, "protocol"), "vless", "vmess", "trojan", "hysteria2") {
		return fmt.Errorf("此协议请选择 NodeRS")
	}
	if p.Text(b, "engine") == "noders" && p.Number(b, "machineId") == 0 {
		return fmt.Errorf("请选择接入机器")
	}
	if p.Text(b, "protocol") == "shadowsocks" && p.Text(b, "engine") == "sing-box" {
		return fmt.Errorf("多用户 Shadowsocks 请选择 Xray")
	}
	if p.Number(b, "port") <= 0 || p.Number(b, "port") > 65535 || p.Number(b, "trafficRate") <= 0 {
		return fmt.Errorf("端口或流量倍率无效")
	}
	return nil
}
func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可管理节点")
	}
	res := resource()
	machines, e := a.C.List("machines")
	if e != nil {
		return p.View{}, e
	}
	mf := c.Field("machineId", "接入机器（NodeRS）", "select", false)
	for _, machine := range machines {
		mf.Options = append(mf.Options, p.Option{Label: fmt.Sprintf("%.0f · %s", p.Number(machine, "number"), p.Text(machine, "name")), Value: machine["number"]})
	}
	res.Fields = append(res.Fields, mf)
	for i := range res.Fields {
		switch res.Fields[i].Key {
		case "port":
			res.Fields[i].Default = 443
		case "trafficRate":
			res.Fields[i].Default = 1
		case "engine":
			res.Fields[i].Default = "xray"
		case "protocol":
			res.Fields[i].Default = "vless"
		case "groups":
			res.Fields[i].Default = "default"
		case "enabled", "tls":
			res.Fields[i].Default = true
		}
	}
	rows, e := a.C.List("nodes")
	online := 0
	for _, row := range rows {
		if seen, err := time.Parse(time.RFC3339, p.Text(row, "lastSeen")); err == nil && time.Since(seen) < heartbeatTimeout {
			online++
		}
	}
	return p.View{Title: "节点管理", Metrics: []p.Metric{{Label: "全部节点", Value: len(rows)}, {Label: "在线节点", Value: online}}, Resources: []p.Resource{res}}, e
}
func (a *app) rotate(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.Find("nodes", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	row["agentToken"] = p.ID() + p.ID()
	if !c.Err(w, a.C.Put("nodes", row)) {
		p.JSON(w, p.Record{"token": row["agentToken"], "nodeId": row["id"]})
	}
}
func (a *app) internal(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "subscriptions") {
		return
	}
	rows, e := a.C.List("nodes")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	byID := map[string]p.Record{}
	for _, row := range rows {
		byID[p.Text(row, "id")] = row
	}
	for _, row := range rows {
		if nodeVisible(row) && (p.Bool(row, "enabled") || p.Bool(byID[p.Text(row, "parentId")], "enabled")) {
			delete(row, "agentToken")
			delete(row, "keyPath")
			delete(row, "certPath")
			out = append(out, row)
		}
	}
	p.JSON(w, out)
}

func nodeVisible(row p.Record) bool {
	visible, exists := row["visible"]
	return !exists || visible != false
}

func (a *app) sync(w http.ResponseWriter, r *http.Request) {
	var b struct {
		NodeID      string     `json:"nodeId"`
		ReportID    string     `json:"reportId"`
		Usage       []p.Record `json:"usage"`
		Connections []p.Record `json:"connections"`
		Error       string     `json:"error"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	node, e := a.Find("nodes", b.NodeID)
	if c.Err(w, e) {
		return
	}
	if r.Header.Get("X-Node-Token") != p.Text(node, "agentToken") {
		p.Fail(w, 401, "节点接入令牌无效")
		return
	}
	if !p.Bool(node, "enabled") {
		p.JSON(w, p.Record{"enabled": false})
		return
	}
	if b.ReportID != "" {
		done, e := a.hasReport(b.ReportID)
		if c.Err(w, e) {
			return
		}
		if !done {
			if len(b.Usage) > 0 {
				if e = a.C.Emit("traffic.reported", p.Record{"nodeId": b.NodeID, "usage": b.Usage, "rate": node["trafficRate"], "reportId": b.ReportID}); c.Err(w, e) {
					return
				}
			}
			if len(b.Connections) > 0 {
				if e = a.C.Emit("traffic.observed", p.Record{"nodeId": b.NodeID, "reportId": b.ReportID, "connections": b.Connections}); c.Err(w, e) {
					return
				}
			}
			if c.Err(w, a.C.Put("reports", p.Record{"id": b.ReportID, "createdAt": c.Now()})) {
				return
			}
		}
	}
	node["lastSeen"] = c.Now()
	node["error"] = b.Error
	if c.Err(w, a.C.Put("nodes", node)) {
		return
	}
	var clients []p.Record
	if c.Err(w, a.C.Call("access", "GET", "/internal/clients", nil, &clients)) {
		return
	}
	filtered := []p.Record{}
	for _, client := range clients {
		if c.CanUse(client, node) {
			filtered = append(filtered, client)
		}
	}
	var rules []p.Record
	_ = a.C.Call("audit", "GET", "/internal/rules", nil, &rules)
	p.JSON(w, p.Record{"enabled": true, "node": node, "clients": filtered, "rules": rules})
}

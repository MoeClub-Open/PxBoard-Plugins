package nodes

import (
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct {
	*c.App
	machineMu sync.Mutex
}

func New() http.Handler {
	a := &app{App: c.New()}
	a.machineAPI()
	a.monitoring()
	a.settingsAPI()
	a.Mux.HandleFunc("GET /groups", a.groups)
	a.Mux.HandleFunc("POST /groups", a.groups)
	a.Mux.HandleFunc("PATCH /groups/{id}", a.groups)
	a.Mux.HandleFunc("GET /groups/options", a.groupOptions)
	a.nodesAPI()
	a.View("nodes", a.view)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		if !p.Admin(r) {
			return nil, fmt.Errorf("无权查看节点概况")
		}
		rows, e := a.C.List("nodes")
		if e != nil {
			return nil, e
		}
		machines, e := a.C.List("machines")
		return []p.Metric{
			{Label: "在线服务器", Value: onlineServerCount(machines, time.Now())},
			{Label: "全部服务器", Value: len(machines)},
			{Label: "主节点", Value: len(primaryNodes(rows))},
		}, e
	})
	a.Mux.HandleFunc("GET /internal/nodes", a.internal)
	return a
}

func (a *app) nodesAPI() {
	a.Mux.HandleFunc("GET /nodes", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		rows, e := a.C.List("nodes")
		if c.Err(w, e) {
			return
		}
		out := make([]p.Record, 0, len(rows))
		for _, row := range rows {
			out = append(out, materializeFluxNodeForView(row))
		}
		p.JSON(w, out)
	})
	a.Mux.HandleFunc("POST /nodes", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		delete(b, "id")
		if c.Err(w, a.normalize(r, b, nil)) {
			return
		}
		cleanRecordNilKeys(b)
		b["createdAt"] = c.Now()
		if !c.Err(w, a.C.Put("nodes", b)) {
			p.JSON(w, b)
		}
	})
	a.Mux.HandleFunc("PATCH /nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		old, e := a.Find("nodes", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		var b p.Record
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		b["id"] = old["id"]
		if c.Err(w, a.normalize(r, b, old)) {
			return
		}
		for k, v := range b {
			if v == nil {
				delete(old, k)
			} else {
				old[k] = v
			}
		}
		if !c.Err(w, a.C.Put("nodes", old)) {
			p.JSON(w, old)
		}
	})
	a.Mux.HandleFunc("DELETE /nodes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !c.Require(w, r) {
			return
		}
		if !c.Err(w, a.C.Delete("nodes", r.PathValue("id"))) {
			p.JSON(w, true)
		}
	})
}

func cleanRecordNilKeys(b p.Record) {
	for k, v := range b {
		if v == nil {
			delete(b, k)
		}
	}
}

func (a *app) normalize(r *http.Request, b, old p.Record) error {
	_, hostProvided := b["host"]
	_, portProvided := b["port"]
	transportChanged := b["transport"] != nil
	_, certificatePatched := b["certConfig"]
	mergeNodeSettings(b, old)
	certificateChanged := certificatePatched && !reflect.DeepEqual(b["certConfig"], old["certConfig"])
	c.Defaults(b, old, old)
	if b["certConfig"] == nil && (p.Text(b, "certPath") != "" || p.Text(b, "keyPath") != "") {
		b["certConfig"] = map[string]any{"mode": "file", "cert_path": p.Text(b, "certPath"), "key_path": p.Text(b, "keyPath")}
	}
	if old == nil {
		number, e := a.nextNumber("nodes", "wireId")
		if e != nil {
			return e
		}
		b["wireId"] = number
	} else {
		b["wireId"] = old["wireId"]
	}
	protocol := p.Text(b, "protocol")
	tls := protocol != "mieru" && protocol != "shadowsocks" && protocol != "sudoku" && protocol != "flux"
	c.Defaults(b, old, p.Record{"protocol": "anytls", "port": 443.0, "trafficRate": 1.0, "enabled": true, "visible": true, "tls": tls, "groups": "default"})
	protocol = p.Text(b, "protocol")
	if e := c.Validate(b, "name", "protocol"); e != nil {
		return e
	}
	if !c.Allowed(protocol, "vless", "vmess", "trojan", "shadowsocks", "hysteria2", "anytls", "tuic", "mieru", "sudoku", "naive", "flux") {
		return fmt.Errorf("协议无效")
	}
	if p.Number(b, "machineId") == 0 {
		return fmt.Errorf("请选择所属服务器")
	}
	if p.Number(b, "trafficRate") <= 0 {
		return fmt.Errorf("流量倍率无效")
	}
	if protocol != "flux" {
		if e := c.Validate(b, "host"); e != nil {
			return e
		}
		if p.Number(b, "port") <= 0 || p.Number(b, "port") > 65535 {
			return fmt.Errorf("连接端口无效")
		}
	}
	if err := normalizeNodeConnection(b, old == nil, transportChanged, certificateChanged, hostProvided, portProvided); err != nil {
		return err
	}
	if e := c.Validate(b, "host"); e != nil {
		return e
	}
	if p.Number(b, "port") <= 0 || p.Number(b, "port") > 65535 {
		return fmt.Errorf("连接端口无效")
	}
	b["engine"] = nil
	b["agentToken"] = nil
	b["certPath"] = nil
	b["keyPath"] = nil
	return nil
}
func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可管理节点")
	}
	machines, e := a.C.List("machines")
	if e != nil {
		return p.View{}, e
	}
	sortServers(machines)
	serverOptions := make([]p.Option, 0, len(machines))
	for _, machine := range machines {
		serverOptions = append(serverOptions, p.Option{Label: fmt.Sprintf("%.0f · %s", p.Number(machine, "number"), p.Text(machine, "name")), Value: machine["number"]})
	}
	res := resource(serverOptions)
	rows, e := a.C.List("nodes")
	return p.View{
		Title:     "节点管理",
		Metrics:   []p.Metric{{Label: "主节点", Value: len(primaryNodes(rows))}, {Label: "连接地址", Value: len(rows)}, {Label: "关联服务器", Value: len(machines)}},
		Resources: []p.Resource{res, groupResource()},
	}, e
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
		parent := byID[p.Text(row, "parentId")]
		if nodeVisible(row) && (p.Bool(row, "enabled") || p.Bool(parent, "enabled")) {
			row = subscriptionNode(row, parent)
			if p.Text(row, "protocol") == "flux" {
				row = sanitizeFluxSubscriptionNode(row)
			}
			delete(row, "agentToken")
			delete(row, "keyPath")
			delete(row, "certPath")
			out = append(out, row)
		}
	}
	p.JSON(w, out)
}

func subscriptionNode(node, parent p.Record) p.Record {
	result := p.Record{}
	for key, value := range node {
		result[key] = value
	}
	if parent != nil && !p.Bool(node, "enabled") {
		for _, key := range []string{"protocol", "passwordMode", "tls", "tlsMode", "serverName", "tlsSettings", "realitySettings", "protocolOptions", "transport", "transportSettings"} {
			result[key] = parent[key]
		}
		result["parentAccess"] = p.Record{"groups": parent["groups"], "minimumAccessLevel": parent["minimumAccessLevel"], "allowedAccountIds": parent["allowedAccountIds"]}
	}
	return result
}

func nodeVisible(row p.Record) bool {
	visible, exists := row["visible"]
	return !exists || visible != false
}

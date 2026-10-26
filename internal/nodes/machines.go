package nodes

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"strconv"
)

func (a *app) nextNumber(collection, key string) (float64, error) {
	rows, e := a.C.List(collection)
	value := 0.0
	for _, row := range rows {
		value = max(value, p.Number(row, key))
	}
	return value + 1, e
}
func (a *app) machinesView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可管理机器")
	}
	return p.View{Title: "机器", Resources: []p.Resource{{Key: "machines", Title: "机器", Endpoint: "/machines", CreateLabel: "添加机器", Editable: true, Deletable: true, Columns: []p.Field{c.Field("number", "机器编号", "number", false), c.Field("name", "名称", "text", false), c.Field("lastSeen", "最近在线", "datetime", false), c.Field("enabled", "启用", "boolean", false)}, Fields: []p.Field{c.Field("name", "名称", "text", true), {Key: "enabled", Label: "启用", Type: "boolean", Default: true}}, Actions: []p.Action{{Key: "credentials", Label: "接入凭据"}, {Key: "rotate", Label: "轮换密钥", Confirm: "轮换后原机器密钥立即失效，需要更新 NodeRS 配置。"}}}}}, nil
}
func (a *app) machineCredentials(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.Find("machines", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if r.PathValue("action") == "rotate" {
		row["key"] = p.ID() + p.ID()
		if c.Err(w, a.C.Put("machines", row)) {
			return
		}
	} else if r.PathValue("action") != "credentials" {
		p.Fail(w, 404, "操作不存在")
		return
	}
	p.JSON(w, p.Record{"token": row["key"], "identifier": row["number"], "identifierLabel": "机器编号"})
}
func (a *app) machine(w http.ResponseWriter, r *http.Request) (p.Record, bool) {
	number, e := strconv.ParseInt(r.PathValue("machine"), 10, 64)
	if e != nil {
		p.Fail(w, 400, "机器编号无效")
		return nil, false
	}
	rows, e := a.C.List("machines")
	if c.Err(w, e) {
		return nil, false
	}
	for _, row := range rows {
		if int64(p.Number(row, "number")) == number && p.Bool(row, "enabled") && r.Header.Get("X-Machine-Key") == p.Text(row, "key") && p.Text(row, "key") != "" {
			return row, true
		}
	}
	p.Fail(w, 401, "机器凭据无效或机器已停用")
	return nil, false
}
func (a *app) machineNode(w http.ResponseWriter, r *http.Request, machine p.Record) (p.Record, bool) {
	number, e := strconv.ParseInt(r.PathValue("node"), 10, 64)
	if e != nil {
		p.Fail(w, 400, "节点编号无效")
		return nil, false
	}
	rows, e := a.C.List("nodes")
	if c.Err(w, e) {
		return nil, false
	}
	for _, row := range rows {
		if int64(p.Number(row, "wireId")) == number && row["machineId"] == machine["number"] && p.Bool(row, "enabled") && p.Text(row, "engine") == "noders" {
			return row, true
		}
	}
	p.Fail(w, 404, "节点不属于此机器或已停用")
	return nil, false
}
func (a *app) machineAPI() {
	a.CRUD(p.Resource{Key: "machines", Endpoint: "/machines", CreateLabel: "添加机器", Editable: true, Deletable: true}, func(r *http.Request, b, old p.Record) error {
		if old == nil {
			number, e := a.nextNumber("machines", "number")
			if e != nil {
				return e
			}
			b["number"] = number
			b["key"] = p.ID() + p.ID()
		} else {
			b["number"] = old["number"]
			b["key"] = old["key"]
		}
		c.Defaults(b, old, p.Record{"enabled": true})
		return c.Validate(b, "name")
	})
	a.View("machines", a.machinesView)
	a.Mux.HandleFunc("POST /machines/{id}/{action}", a.machineCredentials)
	base := "/machines/{machine}"
	a.Mux.HandleFunc("GET "+base+"/handshake", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.machine(w, r); ok {
			p.JSON(w, p.Record{"apiVersion": "1.0", "transport": "poll", "capabilities": []string{"multiNode", "trafficOutbox", "onlineAccounts"}})
		}
	})
	a.Mux.HandleFunc("GET "+base+"/nodes", a.machineNodes)
	a.Mux.HandleFunc("GET "+base+"/nodes/{node}/config", a.machineConfig)
	a.Mux.HandleFunc("GET "+base+"/nodes/{node}/users", a.machineUsers)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/traffic", a.machineTraffic)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/alive", a.machineTelemetry)
	a.Mux.HandleFunc("POST "+base+"/nodes/{node}/status", a.machineTelemetry)
	a.Mux.HandleFunc("POST "+base+"/status", a.machineTelemetry)
}

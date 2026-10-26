package nodes

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const machineConfigurations = "machineConfigurations"

type managedPanel struct {
	Type      string `json:"type"`
	API       string `json:"api"`
	Key       string `json:"key"`
	MachineID int64  `json:"machine_id"`
}

func (a *app) configurationAPI(base string) {
	a.Mux.HandleFunc("GET "+base+"/configuration", a.pullMachineConfiguration)
	a.Mux.HandleFunc("POST "+base+"/configuration/status", a.machineConfigurationStatus)
	a.Mux.HandleFunc("GET /machine-configurations", a.listMachineConfigurations)
	a.Mux.HandleFunc("POST /machine-configurations/{id}/retry", a.retryMachineConfiguration)
}

func (a *app) configuration(machineID string) (p.Record, error) {
	rows, e := a.C.List(machineConfigurations)
	for _, row := range rows {
		if p.Text(row, "id") == machineID {
			return row, e
		}
	}
	return nil, e
}

func (a *app) configureMachine(w http.ResponseWriter, r *http.Request, machine p.Record) {
	var b struct {
		PanelType string `json:"panelType"`
		API       string `json:"api"`
		Key       string `json:"key"`
		MachineID int64  `json:"machineId"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	api := strings.TrimRight(strings.TrimSpace(b.API), "/")
	parsed, e := url.Parse(api)
	if e != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		p.Fail(w, 400, "请填写完整的目标面板 HTTP 或 HTTPS 地址")
		return
	}
	if !c.Allowed(b.PanelType, "pxboard", "xboard") || b.Key == "" || b.MachineID < 1 {
		p.Fail(w, 400, "目标面板类型、机器编号或密钥无效")
		return
	}
	old, e := a.configuration(p.Text(machine, "id"))
	if c.Err(w, e) {
		return
	}
	if p.Text(old, "status") == "pending" {
		p.Fail(w, 409, "此机器已有等待处理的配置")
		return
	}
	row := p.Record{
		"id": machine["id"], "machineName": machine["name"], "revision": p.Number(old, "revision") + 1,
		"config": p.Record{"panel": managedPanel{b.PanelType, api, b.Key, b.MachineID}},
		"status": "pending", "message": "", "updatedAt": c.Now(),
	}
	if !c.Err(w, a.C.Put(machineConfigurations, row)) {
		p.JSON(w, p.Record{"message": "配置已下发，等待 NodeRS 接收", "revision": row["revision"]})
	}
}

func (a *app) pullMachineConfiguration(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	row, e := a.configuration(p.Text(m, "id"))
	if c.Err(w, e) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if row == nil || p.Text(row, "status") != "pending" {
		p.JSON(w, nil)
		return
	}
	p.JSON(w, p.Record{"revision": row["revision"], "config": row["config"]})
}

func (a *app) machineConfigurationStatus(w http.ResponseWriter, r *http.Request) {
	m, ok := a.machine(w, r)
	if !ok {
		return
	}
	var b struct {
		Revision float64 `json:"revision"`
		Status   string  `json:"status"`
		Message  string  `json:"message"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row, e := a.configuration(p.Text(m, "id"))
	if c.Err(w, e) {
		return
	}
	if row == nil || b.Revision != p.Number(row, "revision") {
		p.Fail(w, 409, "配置版本已变更")
		return
	}
	if !c.Allowed(b.Status, "applied", "failed") {
		p.Fail(w, 400, "配置状态无效")
		return
	}
	if p.Text(row, "status") != "pending" && p.Text(row, "status") != b.Status {
		p.Fail(w, 409, "此配置已经处理")
		return
	}
	config, _ := row["config"].(map[string]any)
	panel, _ := config["panel"].(map[string]any)
	message := b.Message
	if key := p.Text(panel, "key"); key != "" {
		message = strings.ReplaceAll(message, key, "[密钥已隐藏]")
	}
	if key := p.Text(m, "key"); key != "" {
		message = strings.ReplaceAll(message, key, "[密钥已隐藏]")
	}
	row["status"], row["message"], row["updatedAt"] = b.Status, message, c.Now()
	if !c.Err(w, a.C.Put(machineConfigurations, row)) {
		p.JSON(w, true)
	}
}

func (a *app) listMachineConfigurations(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List(machineConfigurations)
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	labels := map[string]string{"pending": "等待节点处理", "applied": "已应用", "failed": "应用失败"}
	for _, row := range rows {
		config, _ := row["config"].(map[string]any)
		panel, _ := config["panel"].(map[string]any)
		out = append(out, p.Record{"id": row["id"], "machineName": row["machineName"], "revision": row["revision"], "status": row["status"], "statusLabel": labels[p.Text(row, "status")], "message": row["message"], "updatedAt": row["updatedAt"], "panelType": panel["type"], "api": panel["api"], "machineId": panel["machine_id"]})
	}
	p.JSON(w, out)
}

func (a *app) retryMachineConfiguration(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, e := a.configuration(r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if row == nil || p.Text(row, "status") != "failed" {
		p.Fail(w, 409, "只有失败的配置可以重新下发")
		return
	}
	row["revision"], row["status"], row["message"], row["updatedAt"] = p.Number(row, "revision")+1, "pending", "", c.Now()
	if !c.Err(w, a.C.Put(machineConfigurations, row)) {
		p.JSON(w, p.Record{"message": fmt.Sprintf("配置版本 %.0f 已重新下发", p.Number(row, "revision"))})
	}
}

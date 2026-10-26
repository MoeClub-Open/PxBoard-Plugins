package audit

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	res := ruleResource()
	a.CRUD(res, func(r *http.Request, b, old p.Record) error {
		c.Defaults(b, old, p.Record{"enabled": true, "kind": "domain", "action": "log"})
		if e := c.Validate(b, "name", "pattern"); e != nil {
			return e
		}
		switch p.Text(b, "kind") {
		case "regex":
			if _, e := regexp.Compile(p.Text(b, "pattern")); e != nil {
				return fmt.Errorf("正则表达式无效：%w", e)
			}
		case "cidr":
			if _, _, e := net.ParseCIDR(p.Text(b, "pattern")); e != nil {
				return fmt.Errorf("IP 网段无效")
			}
		case "domain":
			b["pattern"] = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(p.Text(b, "pattern")), "."))
		case "keyword", "protocol":
		default:
			return fmt.Errorf("匹配类型无效")
		}
		if !c.Allowed(p.Text(b, "action"), "log", "block", "suspend") {
			return fmt.Errorf("处理方式无效")
		}
		return nil
	})
	a.Mux.HandleFunc("GET /logs", a.logs)
	a.Mux.HandleFunc("GET /internal/rules", a.rules)
	a.View("audit", a.view)
	a.Summary(func(r *http.Request) ([]p.Metric, error) {
		if !p.Admin(r) {
			return nil, fmt.Errorf("无权查看审计概况")
		}
		logs, e := a.C.List("logs")
		if e != nil {
			return nil, e
		}
		rules, e := a.C.List("rules")
		return []p.Metric{{Label: "7 天内命中", Value: len(logs)}, {Label: "审计规则", Value: len(rules)}}, e
	})
	a.Events(a.event)
	return a
}
func ruleResource() p.Resource {
	return p.Resource{Key: "rules", Title: "审计规则", Endpoint: "/rules", CreateLabel: "添加规则", Editable: true, Deletable: true, Columns: []p.Field{c.Field("name", "规则名称", "text", false), c.Field("kind", "匹配方式", "tag", false), c.Field("pattern", "匹配内容", "text", false), c.Field("action", "处理方式", "status", false), c.Field("enabled", "启用", "boolean", false)}, Fields: []p.Field{c.Field("name", "规则名称", "text", true), c.Select("kind", "匹配方式", "domain|域名及子域名", "keyword|关键词", "regex|正则表达式", "cidr|IP 网段", "protocol|协议"), c.Field("pattern", "匹配内容", "text", true), c.Select("action", "命中后处理", "log|仅记录", "block|节点阻断", "suspend|暂停账户"), c.Field("enabled", "启用", "boolean", false)}}
}
func (a *app) logs(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("logs")
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) rules(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "nodes") {
		return
	}
	rows, e := a.Filter("rules", func(row p.Record) bool { return p.Bool(row, "enabled") && p.Text(row, "action") == "block" })
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func matches(rule, flow p.Record) bool {
	pattern := strings.ToLower(p.Text(rule, "pattern"))
	host := strings.ToLower(strings.TrimSuffix(p.Text(flow, "host"), "."))
	switch p.Text(rule, "kind") {
	case "domain":
		pattern = strings.TrimSuffix(strings.TrimSpace(pattern), ".")
		return host == pattern || strings.HasSuffix(host, "."+pattern)
	case "keyword":
		return strings.Contains(host, pattern)
	case "regex":
		re, e := regexp.Compile(p.Text(rule, "pattern"))
		return e == nil && re.MatchString(host)
	case "protocol":
		return strings.EqualFold(p.Text(flow, "protocol"), pattern)
	case "cidr":
		_, network, e := net.ParseCIDR(pattern)
		return e == nil && network.Contains(net.ParseIP(p.Text(flow, "ip")))
	}
	return false
}
func (a *app) event(e p.Event) error {
	switch e.Type {
	case "traffic.observed":
		if e.Source != "nodes" {
			return fmt.Errorf("连接记录来源无效")
		}
		rules, err := a.C.List("rules")
		if err != nil {
			return err
		}
		connections, _ := e.Data["connections"].([]any)
		for i, raw := range connections {
			connection, _ := raw.(map[string]any)
			for _, rule := range rules {
				if !p.Bool(rule, "enabled") || !matches(rule, connection) {
					continue
				}
				flow := p.Record{"id": fmt.Sprintf("%s-%d-%s", p.Text(e.Data, "reportId"), i, p.Text(rule, "id")), "userId": connection["userId"], "nodeId": e.Data["nodeId"], "host": connection["host"], "protocol": connection["protocol"], "ruleName": rule["name"], "action": rule["action"], "createdAt": e.CreatedAt}
				if err = a.C.Put("logs", flow); err != nil {
					return err
				}
				if p.Text(rule, "action") == "suspend" {
					if err = a.C.Emit("access.suspend", p.Record{"userId": flow["userId"], "reason": "审计命中：" + p.Text(rule, "name")}); err != nil {
						return err
					}
				}
			}
		}
	case "system.tick":
		rows, err := a.C.List("logs")
		if err != nil {
			return err
		}
		cut := time.Now().AddDate(0, 0, -7).UTC().Format(time.RFC3339)
		for _, row := range rows {
			if p.Text(row, "createdAt") < cut {
				if err = a.C.Delete("logs", p.Text(row, "id")); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (a *app) view(r *http.Request) (p.View, error) {
	rows, e := a.C.List("logs")
	return p.View{Title: "流量审计", Notice: "记录连接域名、协议与规则命中，不采集通信正文。命中记录保留 7 天；阻断规则在节点下次同步后生效。", Metrics: []p.Metric{{Label: "最近 7 天命中", Value: len(rows)}}, Resources: []p.Resource{ruleResource(), {Key: "logs", Title: "命中记录", Endpoint: "/logs", Columns: []p.Field{c.Field("createdAt", "时间", "datetime", false), c.Field("userId", "用户 ID", "text", false), c.Field("host", "目标域名", "text", false), c.Field("ruleName", "规则", "text", false), c.Field("action", "处理方式", "status", false)}}}}, e
}

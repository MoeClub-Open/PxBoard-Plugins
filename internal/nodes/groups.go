package nodes

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func groupResource() p.Resource {
	return p.Resource{Key: "groups", Title: "节点分组", Endpoint: "/groups", CreateLabel: "添加分组", Editable: true,
		Fields:  []p.Field{c.Field("name", "分组名称", "text", true), {Key: "enabled", Label: "可用于新增选择", Type: "boolean", Default: true, Hint: "关闭后现有关联继续生效；默认分组始终可用"}},
		Columns: []p.Field{c.Field("name", "分组名称", "text", false), c.Field("sourceId", "来源", "text", false), c.Field("nodeCount", "节点数", "number", false), c.Field("enabled", "可用于新增选择", "boolean", false)}}
}

func splitGroups(value string) []string {
	var result []string
	for _, id := range strings.Split(value, ",") {
		if id = strings.TrimSpace(id); id != "" {
			result = append(result, id)
		}
	}
	return result
}

func (a *app) groupList() ([]p.Record, error) {
	groups, err := a.C.List("groups")
	if err != nil {
		return nil, err
	}
	byID := map[string]p.Record{}
	for _, group := range groups {
		byID[p.Text(group, "id")] = group
	}
	if byID["default"] == nil {
		byID["default"] = p.Record{"id": "default", "name": "默认分组", "enabled": true}
	}
	for _, group := range byID {
		group["nodeCount"] = 0.0
	}
	nodes, err := a.C.List("nodes")
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		for _, id := range splitGroups(p.Text(node, "groups")) {
			if byID[id] == nil {
				byID[id] = p.Record{"id": id, "name": id, "enabled": true}
			}
			byID[id]["nodeCount"] = p.Number(byID[id], "nodeCount") + 1
		}
	}
	groups = groups[:0]
	for _, group := range byID {
		if group["enabled"] == nil {
			group["enabled"] = true
		}
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return p.Text(groups[i], "name") < p.Text(groups[j], "name") })
	return groups, nil
}

func (a *app) groups(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		groups, err := a.groupList()
		if !c.Err(w, err) {
			p.JSON(w, groups)
		}
		return
	}
	var body p.Record
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	var group p.Record
	if r.Method == http.MethodPatch {
		groups, err := a.groupList()
		if c.Err(w, err) {
			return
		}
		for _, candidate := range groups {
			if p.Text(candidate, "id") == r.PathValue("id") {
				group = candidate
				break
			}
		}
		if group == nil {
			p.Fail(w, 404, "分组不存在")
			return
		}
	} else {
		group = p.Record{"id": p.ID(), "enabled": true, "createdAt": c.Now()}
	}
	for _, key := range []string{"name", "enabled"} {
		if value, ok := body[key]; ok {
			group[key] = value
		}
	}
	group["name"] = strings.TrimSpace(p.Text(group, "name"))
	if p.Text(group, "name") == "" {
		p.Fail(w, 400, "请输入分组名称")
		return
	}
	if _, ok := group["enabled"].(bool); !ok {
		p.Fail(w, 400, "分组状态无效")
		return
	}
	if p.Text(group, "id") == "default" && !p.Bool(group, "enabled") {
		p.Fail(w, 400, "默认分组不能关闭")
		return
	}
	delete(group, "nodeCount")
	if !c.Err(w, a.C.Put("groups", group)) {
		p.JSON(w, group)
	}
}

func (a *app) groupOptions(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	groups, err := a.groupList()
	if c.Err(w, err) {
		return
	}
	allowed := map[string]bool{}
	all := p.Admin(r)
	if !all {
		var account p.Record
		if c.Err(w, a.C.Call("access", "GET", "/internal/accounts/"+url.PathEscape(p.Subject(r).ID), nil, &account)) {
			return
		}
		all = p.Bool(account, "allNodeGroups")
		for _, id := range splitGroups(p.Text(account, "groups")) {
			allowed[id] = true
		}
	}
	options := []p.Record{}
	counts := map[string]int{}
	for _, group := range groups {
		counts[strings.ToLower(p.Text(group, "name"))]++
	}
	for _, group := range groups {
		if all || allowed[p.Text(group, "id")] {
			name := p.Text(group, "name")
			if counts[strings.ToLower(name)] > 1 {
				source := p.Text(group, "sourceId")
				if source == "" {
					source = p.Text(group, "id")
				}
				name += "（" + source + "）"
			}
			options = append(options, p.Record{"id": group["id"], "name": name, "enabled": group["enabled"]})
		}
		delete(allowed, p.Text(group, "id"))
	}
	for id := range allowed {
		options = append(options, p.Record{"id": id, "name": fmt.Sprintf("未命名分组（%s）", id), "enabled": true})
	}
	p.JSON(w, options)
}

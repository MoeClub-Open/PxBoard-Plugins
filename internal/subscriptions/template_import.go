package subscriptions

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

var templateSourceID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,48}$`)

func (a *app) importTemplates(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var body struct {
		SourceID  string `json:"sourceId"`
		Activate  bool   `json:"activate"`
		Templates []struct {
			Name    string `json:"name"`
			Content string `json:"content"`
		} `json:"templates"`
	}
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	if body.SourceID == "" {
		body.SourceID = "xboard"
	}
	if !templateSourceID.MatchString(body.SourceID) {
		p.Fail(w, 400, "模板来源标识无效")
		return
	}
	settings, err := a.templateSettings()
	if c.Err(w, err) {
		return
	}
	writes := []p.Write{}
	preferred := map[string]string{}
	warnings := []string{}
	ids := []string{}
	seen := map[string]bool{}
	for _, input := range body.Templates {
		name := strings.ToLower(input.Name)
		if !c.Allowed(name, "clash", "clashmeta", "singbox", "sing-box") {
			warnings = append(warnings, "未导入此客户端格式："+name)
			continue
		}
		id := body.SourceID + ":" + name
		if seen[id] {
			p.Fail(w, 400, "同一来源的模板名称重复")
			return
		}
		seen[id] = true
		format, content, conversionErr := sc.Xboard(name, input.Content)
		valid := conversionErr == nil
		if content == "" {
			content = input.Content
		}
		validationError := ""
		if conversionErr != nil {
			validationError = conversionErr.Error()
			warnings = append(warnings, "模板 "+name+" 保留为停用草稿："+validationError)
		}
		prior, err := a.C.Get("templates", id)
		if err == nil {
			if p.Text(prior, "originalContent") != input.Content {
				p.Fail(w, 409, "目标已有不同来源内容的同名模板，请使用新的来源标识")
				return
			}
			valid = p.Bool(prior, "enabled") && sc.Check(p.Text(prior, "format"), p.Text(prior, "content")) == nil
		} else {
			var apiError *p.APIError
			if !errors.As(err, &apiError) || apiError.Status != 404 {
				c.Err(w, err)
				return
			}
			writes = append(writes, p.Write{Collection: "templates", Record: p.Record{"id": id, "name": "Xboard / " + input.Name, "format": format, "content": content, "originalContent": input.Content, "sourceId": body.SourceID, "enabled": valid, "validationError": validationError, "createdAt": c.Now()}})
		}
		if valid && (preferred[format] == "" || name == "clashmeta") {
			preferred[format] = id
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		p.Fail(w, 400, "没有可导入的 Mihomo 或 sing-box 模板")
		return
	}
	if body.Activate {
		for format, id := range preferred {
			settings[templateKey(format)] = id
		}
		writes = append(writes, p.Write{Collection: "settings", Record: settings})
	}
	if c.Err(w, a.C.Commit("template-import:"+p.ID(), writes)) {
		return
	}
	p.JSON(w, p.Record{"ids": ids, "warnings": warnings, "activated": body.Activate && len(preferred) > 0, "message": "模板导入完成"})
}

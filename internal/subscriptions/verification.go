package subscriptions

import (
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

// A new version revokes all previous grants without changing subscription URLs.
func (a *app) verificationPolicy() (p.Record, error) {
	rows, err := a.C.List("verification")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			return row, nil
		}
	}
	return p.Record{"id": "settings", "enabled": false}, nil
}

func verified(link, policy p.Record) bool {
	return p.Bool(policy, "enabled") && p.Text(policy, "version") != "" && p.Text(link, "verificationVersion") == p.Text(policy, "version")
}

func (a *app) verificationSettings(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	policy, err := a.verificationPolicy()
	if c.Err(w, err) {
		return
	}
	if r.Method == http.MethodGet {
		public := p.Record{"enabled": p.Bool(policy, "enabled")}
		if r.URL.Path == "/verification/settings" {
			if !p.Admin(r) {
				p.Fail(w, 403, "仅管理员可修改订阅验证")
				return
			}
			p.JSON(w, []p.Record{public})
		} else {
			p.JSON(w, public)
		}
		return
	}
	if !p.Admin(r) {
		p.Fail(w, 403, "仅管理员可修改订阅验证")
		return
	}
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	if body.Enabled == nil {
		p.Fail(w, 400, "请选择是否启用验证")
		return
	}
	if *body.Enabled && !p.Bool(policy, "enabled") {
		var config p.Record
		if err := a.C.Request("GET", "/api/v1/runtime/challenge", nil, &config); err != nil || !p.Bool(config, "enabled") {
			p.Fail(w, 400, "请先安装并启用人机验证插件，完成服务配置")
			return
		}
		policy["version"] = p.ID()
	}
	policy["enabled"] = *body.Enabled
	if !c.Err(w, a.C.Put("verification", policy)) {
		p.JSON(w, p.Record{"enabled": *body.Enabled})
	}
}

func (a *app) verifyLink(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	rows, err := a.queryLinks([]p.Record{{"field": "id", "value": r.PathValue("id")}})
	if c.Err(w, err) {
		return
	}
	if len(rows) == 0 {
		p.Fail(w, 404, "订阅链接不存在")
		return
	}
	link := rows[0]
	if p.Text(link, "userId") != p.Subject(r).ID {
		p.Fail(w, 403, "只能验证自己的订阅")
		return
	}
	policy, err := a.verificationPolicy()
	if c.Err(w, err) {
		return
	}
	if !p.Bool(policy, "enabled") {
		p.Fail(w, 400, "订阅验证未启用")
		return
	}
	var body struct {
		ChallengeToken string `json:"challengeToken"`
	}
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	if body.ChallengeToken == "" {
		p.Fail(w, 400, "请先完成人机验证")
		return
	}
	if err := a.C.Request("POST", "/api/v1/runtime/challenge/verify", p.Record{"action": "subscription", "token": body.ChallengeToken}, nil); err != nil {
		p.Fail(w, 400, "人机验证失败或服务不可用，请重试")
		return
	}
	link["verificationVersion"] = p.Text(policy, "version")
	if !c.Err(w, a.C.Put("links", link)) {
		p.JSON(w, true)
	}
}

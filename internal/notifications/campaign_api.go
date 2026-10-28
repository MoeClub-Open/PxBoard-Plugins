package notifications

import (
	"fmt"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) campaignList(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	result, err := a.ReadRecords("campaigns", r, nil, "subject", "status")
	if c.Err(w, err) {
		return
	}
	for i, row := range result.Items {
		result.Items[i], err = a.campaignSummary(row)
		if c.Err(w, err) {
			return
		}
	}
	c.ListResponse(w, r, result)
}

func (a *app) getCampaign(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	row, err := a.Find("campaigns", r.PathValue("id"))
	if c.Err(w, err) {
		return
	}
	out, err := a.campaignSummary(row)
	if !c.Err(w, err) {
		p.JSON(w, out)
	}
}

func (a *app) saveCampaign(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	a.campaigns.Lock()
	defer a.campaigns.Unlock()
	var b struct{ Subject, Body, Audience, Emails, Search string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row := p.Record{"id": p.ID(), "createdAt": c.Now()}
	if r.Method == "PATCH" {
		var err error
		row, err = a.Find("campaigns", r.PathValue("id"))
		if c.Err(w, err) {
			return
		}
		if !c.Allowed(p.Text(row, "status"), "draft", "ready") {
			p.Fail(w, 400, "已开始的任务不能编辑")
			return
		}
	}
	if strings.TrimSpace(b.Subject) == "" || strings.ContainsAny(b.Subject, "\r\n") || strings.TrimSpace(b.Body) == "" || !c.Allowed(b.Audience, "all", "selected") {
		p.Fail(w, 400, "请填写主题、正文并选择收件范围")
		return
	}
	row["subject"], row["body"], row["audience"], row["emails"], row["search"] = b.Subject, b.Body, b.Audience, b.Emails, b.Search
	row["status"], row["queued"], row["total"], row["updatedAt"] = "draft", 0, 0, c.Now()
	delete(row, "recipients")
	delete(row, "previewedAt")
	if !c.Err(w, a.C.Put("campaigns", row)) {
		p.JSON(w, row)
	}
}

func (a *app) campaignAction(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	a.campaigns.Lock()
	defer a.campaigns.Unlock()
	row, err := a.Find("campaigns", r.PathValue("id"))
	if c.Err(w, err) {
		return
	}
	status, action := p.Text(row, "status"), r.PathValue("action")
	switch action {
	case "preview":
		if !c.Allowed(status, "draft", "ready") {
			p.Fail(w, 400, "只能预览尚未开始的任务")
			return
		}
		users, e := a.campaignTargets(row)
		if c.Err(w, e) {
			return
		}
		if len(users) == 0 {
			p.Fail(w, 400, "没有符合条件的收件人")
			return
		}
		template, err := a.loadTemplate("general")
		if c.Err(w, err) {
			return
		}
		row["mailTemplate"] = map[string]any{"subject": template.Subject, "text": template.Text, "html": template.HTML}
		ids, samples := []string{}, []string{}
		for _, user := range users {
			ids = append(ids, p.Text(user, "id"))
			if len(samples) < 20 {
				samples = append(samples, p.Text(user, "email"))
			}
		}
		row["recipients"], row["total"], row["previewedAt"], row["status"] = ids, len(ids), c.Now(), "ready"
		mail, e := a.campaignMail(row, p.Record{"name": "示例用户", "email": "preview@example.invalid"})
		if c.Err(w, e) || c.Err(w, a.C.Put("campaigns", row)) {
			return
		}
		p.JSON(w, p.Record{"total": len(ids), "sampleEmails": samples, "subject": mail["subject"], "text": mail["body"], "html": mail["html"]})
		return
	case "start":
		if status != "ready" {
			p.Fail(w, 400, "请先预览并确认收件人")
			return
		}
		var b struct{ Total int }
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if b.Total != len(campaignIDs(row)) {
			p.Fail(w, 409, "收件人预览已变化，请重新确认")
			return
		}
		configs, e := a.C.List("mail")
		if c.Err(w, e) {
			return
		}
		if len(configs) == 0 || !p.Bool(configs[0], "enabled") {
			p.Fail(w, 400, "请先配置并启用邮件服务")
			return
		}
		row["status"], row["startedAt"] = "running", c.Now()
	case "pause":
		if status != "running" {
			p.Fail(w, 400, "只有正在发送的任务可以暂停")
			return
		}
		row["status"] = "paused"
	case "resume":
		if status != "paused" {
			p.Fail(w, 400, "只有暂停的任务可以继续")
			return
		}
		row["status"] = "running"
	case "cancel":
		if !c.Allowed(status, "draft", "ready", "running", "paused") {
			p.Fail(w, 400, "任务已结束")
			return
		}
		row["status"] = "canceled"
	default:
		p.Fail(w, 404, fmt.Sprintf("未知操作：%s", action))
		return
	}
	row["updatedAt"] = c.Now()
	if !c.Err(w, a.C.Put("campaigns", row)) {
		p.JSON(w, true)
	}
}

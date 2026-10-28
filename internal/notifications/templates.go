package notifications

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
)

type mailTemplate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Subject    string `json:"subject"`
	Text       string `json:"text"`
	HTML       string `json:"html"`
	Customized bool   `json:"customized"`
	UpdatedAt  string `json:"updatedAt,omitempty"`
}

type templateData struct {
	SiteName, UserName, Email, PublicURL string
	OrderID, OrderNumber, PlanName       string
	TicketID, TicketSubject, ResetURL    string
	Subject, Message, Content            string
	ActionURL, ActionLabel               string
	ExpiryMinutes                        int
}

type renderedMail struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

const defaultHTML = `<!doctype html><html><head><meta charset="utf-8"></head><body style="margin:0;background:#f5f6f8;font-family:Arial,sans-serif;color:#1f2937"><div style="max-width:600px;margin:24px auto;padding:28px;background:#fff;border:1px solid #e5e7eb;border-radius:8px"><div style="font-size:14px;color:#6b7280">{{.SiteName}}</div><h1 style="font-size:22px;margin:18px 0">{{.Subject}}</h1><div style="white-space:pre-wrap;line-height:1.8">{{.Content}}</div>{{if .ActionURL}}<p style="margin-top:24px"><a href="{{.ActionURL}}" style="display:inline-block;background:#2563eb;color:#fff;padding:11px 20px;text-decoration:none;border-radius:5px">{{.ActionLabel}}</a></p>{{end}}</div></body></html>`

var templateDefaults = []mailTemplate{
	{ID: "password-reset", Name: "找回密码", Subject: "{{.SiteName}} · 重置登录密码", Text: "{{.UserName}}，您好：\n\n请在 {{.ExpiryMinutes}} 分钟内打开以下链接重置密码：\n{{.ResetURL}}\n\n如果不是您本人操作，请忽略此邮件。", HTML: defaultHTML},
	{ID: "order-fulfilled", Name: "订单开通", Subject: "{{.SiteName}} · 订单已开通", Text: "{{.UserName}}，您好：\n\n您的订单 {{.OrderNumber}} 已完成。\n商品：{{.PlanName}}\n\n登录查看：{{.ActionURL}}", HTML: defaultHTML},
	{ID: "ticket-replied", Name: "工单回复", Subject: "{{.SiteName}} · 工单有新回复", Text: "{{.UserName}}，您好：\n\n您的工单「{{.TicketSubject}}」已收到回复。\n\n登录查看：{{.ActionURL}}", HTML: defaultHTML},
	{ID: "general", Name: "通用通知", Subject: "{{.Subject}}", Text: "{{.Message}}", HTML: defaultHTML},
}

func defaultTemplate(id string) (mailTemplate, bool) {
	for _, item := range templateDefaults {
		if item.ID == id {
			return item, true
		}
	}
	return mailTemplate{}, false
}

func sampleData() templateData {
	return templateData{SiteName: "PxBoard", UserName: "示例用户", Email: "preview@example.invalid", PublicURL: "https://example.invalid", OrderID: "preview-order", OrderNumber: "PX202610280001", PlanName: "示例套餐 · 月付", TicketID: "preview-ticket", TicketSubject: "连接问题", ResetURL: "https://example.invalid/reset?token=preview-only", Subject: "示例通知", Message: "这是一条通知内容。", ActionURL: "https://example.invalid/user", ActionLabel: "查看详情", ExpiryMinutes: 30}
}

func renderMail(item mailTemplate, data templateData) (renderedMail, error) {
	result := renderedMail{}
	for name, part := range map[string]struct {
		source string
		target *string
	}{
		"subject": {item.Subject, &result.Subject}, "text": {item.Text, &result.Text},
	} {
		t, err := texttemplate.New(name).Option("missingkey=error").Parse(part.source)
		if err != nil {
			return result, err
		}
		var out bytes.Buffer
		if err = t.Execute(&out, data); err != nil {
			return result, err
		}
		*part.target = out.String()
	}
	if strings.TrimSpace(result.Subject) == "" || strings.ContainsAny(result.Subject, "\r\n") || strings.TrimSpace(result.Text) == "" {
		return result, fmt.Errorf("主题和正文不能为空，主题不能包含换行")
	}
	if item.ID == "password-reset" && !strings.Contains(result.Text, data.ResetURL) {
		return result, fmt.Errorf("找回密码正文需保留 {{.ResetURL}} 重置链接")
	}
	if strings.TrimSpace(item.HTML) == "" {
		return result, nil
	}
	data.Subject, data.Content = result.Subject, result.Text
	t, err := htmltemplate.New("html").Option("missingkey=error").Parse(item.HTML)
	if err != nil {
		return result, err
	}
	var out bytes.Buffer
	if err = t.Execute(&out, data); err != nil {
		return result, err
	}
	result.HTML = out.String()
	return result, nil
}

func templateVariables(id string) []map[string]string {
	keys := []string{"SiteName", "UserName", "Email", "PublicURL", "Subject", "Content", "ActionURL", "ActionLabel"}
	switch id {
	case "password-reset":
		keys = append(keys, "ResetURL", "ExpiryMinutes")
	case "order-fulfilled":
		keys = append(keys, "OrderNumber", "OrderID", "PlanName")
	case "ticket-replied":
		keys = append(keys, "TicketSubject", "TicketID")
	case "general":
		keys = append(keys, "Message")
	}
	labels := map[string]string{"SiteName": "站点名称", "UserName": "用户名称", "Email": "用户邮箱", "PublicURL": "面板地址", "Subject": "邮件主题", "Content": "渲染后正文", "ActionURL": "操作链接", "ActionLabel": "操作名称", "ResetURL": "密码重置链接", "ExpiryMinutes": "链接有效分钟数", "OrderNumber": "订单号", "OrderID": "订单标识", "PlanName": "商品名称", "TicketSubject": "工单主题", "TicketID": "工单标识", "Message": "通知内容"}
	out := []map[string]string{}
	for _, key := range keys {
		item := map[string]string{"key": key, "label": labels[key], "token": "{{." + key + "}}"}
		if key == "Content" || key == "Subject" && id != "general" {
			item["scope"] = "html"
		}
		out = append(out, item)
	}
	return out
}

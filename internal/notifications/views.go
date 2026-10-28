package notifications

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

func (a *app) notificationView(r *http.Request) (p.View, error) {
	records := p.Resource{Key: "notifications", Title: "投递记录", Endpoint: "/notifications", Paginated: true,
		Columns: []p.Field{c.Field("to", "收件人", "text", false), c.Field("subject", "主题", "text", false), c.Field("status", "状态", "status", false), c.Field("attempts", "尝试次数", "number", false), c.Field("error", "错误", "text", false), c.Field("createdAt", "创建时间", "datetime", false)},
		Filters: []p.Field{{Key: "status", Label: "投递状态", Type: "select", Options: []p.Option{{Label: "待投递", Value: "pending"}, {Label: "已发送", Value: "sent"}, {Label: "失败", Value: "failed"}, {Label: "已过期", Value: "expired"}}}},
		Actions: []p.Action{{Key: "retry", Label: "重新投递", When: map[string][]string{"status": {"failed"}}}},
	}
	templates := p.Resource{Key: "templates", Title: "邮件模板", Endpoint: "/templates", Renderer: "mail-templates", Editable: true,
		Columns: []p.Field{c.Field("name", "用途", "text", false), c.Field("subject", "主题", "text", false), c.Field("customized", "已自定义", "boolean", false), c.Field("updatedAt", "更新时间", "datetime", false)},
		Fields:  []p.Field{c.Field("subject", "主题", "text", true), c.Field("text", "正文", "textarea", true), c.Field("html", "HTML 外观", "code", false)},
		Actions: []p.Action{{Key: "reset", Label: "恢复默认", Confirm: "恢复此邮件模板的默认内容？"}},
	}
	mail := p.Resource{Key: "mail", Title: "邮件服务", Endpoint: "/mail", Singleton: true, Editable: true,
		Fields:  []p.Field{c.Field("host", "SMTP 服务器", "text", true), {Key: "port", Label: "端口", Type: "number", Default: 587}, c.Field("username", "用户名", "text", false), c.Field("password", "密码（留空保留）", "password", false), c.Field("from", "发件地址", "email", true), c.Field("publicUrl", "面板公开地址", "url", true), {Key: "enabled", Label: "启用通知", Type: "boolean", Default: true}},
		Columns: []p.Field{c.Field("host", "服务器", "text", false), c.Field("from", "发件地址", "text", false), c.Field("enabled", "启用", "boolean", false)},
	}
	return p.View{Title: "邮件通知", Resources: []p.Resource{records, templates, mail}}, nil
}

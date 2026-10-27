package notifications

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

func (a *app) notificationView(r *http.Request) (p.View, error) {
	return p.View{Title: "邮件通知", Resources: []p.Resource{{Key: "mail", Title: "邮件服务", Endpoint: "/mail", CreateLabel: "配置邮件服务", Editable: true, Fields: []p.Field{c.Field("host", "SMTP 服务器", "text", true), {Key: "port", Label: "端口", Type: "number", Default: 587}, c.Field("username", "用户名", "text", false), c.Field("password", "密码（留空保留）", "password", false), c.Field("from", "发件地址", "email", true), c.Field("publicUrl", "面板公开地址", "url", true), {Key: "enabled", Label: "启用通知", Type: "boolean", Default: true}}, Columns: []p.Field{c.Field("host", "服务器", "text", false), c.Field("from", "发件地址", "text", false), c.Field("enabled", "启用", "boolean", false)}}, {Key: "notifications", Title: "投递记录", Endpoint: "/notifications", Columns: []p.Field{c.Field("to", "收件人", "text", false), c.Field("subject", "主题", "text", false), c.Field("status", "状态", "status", false), c.Field("attempts", "尝试次数", "number", false), c.Field("error", "错误", "text", false), c.Field("createdAt", "创建时间", "datetime", false)}, Actions: []p.Action{{Key: "retry", Label: "重新投递", When: map[string][]string{"status": {"failed"}}}}}}}, nil
}

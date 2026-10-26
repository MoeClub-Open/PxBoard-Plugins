package support

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

func contentResource(key string) p.Resource {
	title := "公告"
	if key == "articles" {
		title = "使用文档"
	}
	return p.Resource{Key: key, Title: title, Endpoint: "/" + key, CreateLabel: "发布" + title, Editable: true, Deletable: true, Columns: []p.Field{c.Field("title", "标题", "text", false), c.Field("content", "内容", "longtext", false), c.Field("published", "已发布", "boolean", false), c.Field("createdAt", "创建时间", "datetime", false)}, Fields: []p.Field{c.Field("title", "标题", "text", true), c.Field("content", "内容", "textarea", true), {Key: "published", Label: "发布", Type: "boolean", Default: true}}}
}
func (a *app) ticketView(r *http.Request) (p.View, error) {
	return p.View{Title: "工单", Resources: []p.Resource{{Key: "tickets", Title: "工单记录", Endpoint: "/tickets", CreateLabel: "提交工单", Fields: []p.Field{c.Field("subject", "主题", "text", true), c.Field("content", "问题描述", "textarea", true)}, Columns: []p.Field{c.Field("subject", "主题", "text", false), c.Field("email", "发起人", "text", false), c.Field("status", "状态", "status", false), c.Field("thread", "对话记录", "longtext", false), c.Field("updatedAt", "更新时间", "datetime", false)}, Actions: []p.Action{{Key: "reply", Label: "回复", Fields: []p.Field{c.Field("message", "回复内容", "textarea", true)}}, {Key: "close", Label: "关闭", Confirm: "关闭此工单？"}}}}}, nil
}
func (a *app) contentView(r *http.Request) (p.View, error) {
	resources := []p.Resource{contentResource("announcements"), contentResource("articles")}
	if !p.Admin(r) {
		for i := range resources {
			resources[i].CreateLabel = ""
			resources[i].Editable = false
			resources[i].Deletable = false
		}
	}
	return p.View{Title: "公告与文档", Resources: resources}, nil
}
func (a *app) notificationView(r *http.Request) (p.View, error) {
	return p.View{Title: "邮件通知", Resources: []p.Resource{{Key: "mail", Title: "邮件服务", Endpoint: "/mail", CreateLabel: "配置邮件服务", Editable: true, Fields: []p.Field{c.Field("host", "SMTP 服务器", "text", true), {Key: "port", Label: "端口", Type: "number", Default: 587}, c.Field("username", "用户名", "text", false), c.Field("password", "密码（留空保留）", "password", false), c.Field("from", "发件地址", "email", true), c.Field("publicUrl", "面板公开地址", "url", true), {Key: "enabled", Label: "启用通知", Type: "boolean", Default: true}}, Columns: []p.Field{c.Field("host", "服务器", "text", false), c.Field("from", "发件地址", "text", false), c.Field("enabled", "启用", "boolean", false)}}, {Key: "notifications", Title: "投递记录", Endpoint: "/notifications", Columns: []p.Field{c.Field("to", "收件人", "text", false), c.Field("subject", "主题", "text", false), c.Field("status", "状态", "status", false), c.Field("attempts", "尝试次数", "number", false), c.Field("error", "错误", "text", false), c.Field("createdAt", "创建时间", "datetime", false)}, Actions: []p.Action{{Key: "retry", Label: "重新投递", When: map[string][]string{"status": {"failed"}}}}}}}, nil
}

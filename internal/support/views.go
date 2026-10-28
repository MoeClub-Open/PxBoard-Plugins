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
	return p.View{Title: "工单", Resources: []p.Resource{{Paginated: true, Key: "tickets", Title: "工单记录", Endpoint: "/tickets", CreateLabel: "提交工单", Fields: []p.Field{c.Field("subject", "主题", "text", true), c.Field("content", "问题描述", "textarea", true)}, Columns: []p.Field{c.Field("subject", "主题", "text", false), c.Field("email", "发起人", "text", false), c.Field("status", "状态", "status", false), c.Field("thread", "对话记录", "longtext", false), c.Field("updatedAt", "更新时间", "datetime", false)}, Actions: []p.Action{{Key: "reply", Label: "回复", Fields: []p.Field{c.Field("message", "回复内容", "textarea", true)}}, {Key: "close", Label: "关闭", Confirm: "关闭此工单？"}}}}}, nil
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

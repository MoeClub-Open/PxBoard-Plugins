package subscriptions

import p "github.com/MoeClub-Open/PxBoard/pkg/plugin"

func templateResource() p.Resource {
	return p.Resource{Key: "templates", Title: "订阅模板", Endpoint: "/templates", CreateLabel: "添加模板", Editable: true, Deletable: true,
		Fields:  []p.Field{{Key: "name", Label: "名称", Type: "text", Required: true}, {Key: "format", Label: "格式", Type: "select", Default: "clash", Required: true, Options: []p.Option{{Label: "Mihomo", Value: "clash"}, {Label: "sing-box", Value: "sing-box"}}}, {Key: "enabled", Label: "启用", Type: "boolean", Default: true}, {Key: "content", Label: "模板内容", Type: "code", Hint: "留空使用所选格式的内置模板"}},
		Columns: []p.Field{{Key: "name", Label: "名称"}, {Key: "format", Label: "格式"}, {Key: "enabled", Label: "启用", Type: "boolean"}, {Key: "isDefault", Label: "默认", Type: "boolean"}, {Key: "validationError", Label: "导入待修正项", Type: "longtext"}, {Key: "updatedAt", Label: "更新时间", Type: "datetime"}},
		Actions: []p.Action{{Key: "preview", Label: "预览"}, {Key: "validate", Label: "校验"}, {Key: "default", Label: "设为默认", When: map[string][]string{"enabled": {"true"}, "isDefault": {"false"}}}, {Key: "reset", Label: "恢复内置", Confirm: "使用内置模板替换当前内容？"}}}
}

func templateSettingsResource() p.Resource {
	fields := []p.Field{{Key: "profileTitle", Label: "订阅名称", Type: "text", Hint: "留空使用站点品牌名称"}, {Key: "homepage", Label: "面板地址", Type: "url"}, {Key: "updateIntervalHours", Label: "刷新周期（小时）", Type: "number", Required: true}}
	return p.Resource{Key: "templatePolicy", Title: "订阅内容", Endpoint: "/template-settings", Singleton: true, Editable: true, Fields: fields, Columns: fields}
}

package redeem

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func codeResource() p.Resource {
	return p.Resource{Key: "codes", Title: "兑换码", Endpoint: "/codes", CreateLabel: "创建兑换码", Editable: true, Deletable: true, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("code", "兑换码", "copy", false), c.Field("summary", "兑换内容", "text", false), c.Field("used", "已兑换 / 处理中", "number", false), c.Field("limit", "总可兑换次数", "number", false), c.Field("expiresAt", "到期时间", "datetime", false), c.Field("enabled", "启用", "boolean", false)}}
}

func (a *app) codesView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权管理兑换码")
	}
	resource := codeResource()
	kind := c.Select("kind", "兑换内容", "access|访问权益", "traffic|追加流量", "time|延长期限", "reset|重置流量")
	if a.walletAvailable() {
		kind.Options = append(kind.Options, p.Option{Label: "余额", Value: "balance"})
	}
	kind.Default = "access"
	resource.Fields = []p.Field{
		c.Field("name", "名称", "text", true),
		c.Field("code", "兑换码（留空自动生成）", "text", false),
		kind,
		{Key: "amount", Label: "余额金额", Type: "money", Required: true, When: map[string][]string{"kind": {"balance"}}},
		{Key: "durationUnit", Label: "期限单位", Type: "select", Required: true, Default: "day", Options: []p.Option{{Label: "天", Value: "day"}, {Label: "日历月", Value: "month"}, {Label: "长期", Value: "permanent"}}, When: map[string][]string{"kind": {"access", "time"}}},
		{Key: "durationValue", Label: "期限", Type: "number", Required: true, Default: 30, When: map[string][]string{"kind": {"access", "time"}, "durationUnit": {"day", "month"}}},
		{Key: "quotaGiB", Label: "流量（GB）", Type: "number", Required: true, Default: 20, When: map[string][]string{"kind": {"access", "traffic"}}},
		{Key: "groups", Label: "节点分组", Type: "groups", Required: true, Default: "default", OptionsEndpoint: "/api/v1/extensions/nodes/groups/options", When: map[string][]string{"kind": {"access"}}},
		{Key: "limit", Label: "总可兑换次数（0为不限）", Type: "number", Required: true, Default: 1},
		{Key: "perUserLimit", Label: "每个账号可兑换次数（0为不限）", Type: "number", Required: true, Default: 1},
		c.Field("expiresAt", "兑换码到期时间", "datetime", false),
		{Key: "enabled", Label: "启用", Type: "boolean", Default: true},
	}
	return p.View{Title: "兑换码", Resources: []p.Resource{resource}}, nil
}

func (a *app) redeemView(r *http.Request) (p.View, error) {
	return p.View{Title: "兑换码", Resources: []p.Resource{{Key: "redeem", Title: "兑换", Endpoint: "/redeem", CreateLabel: "兑换", Fields: []p.Field{c.Field("code", "兑换码", "text", true)}, Columns: []p.Field{}}, claimResource(false)}}, nil
}

func claimResource(admin bool) p.Resource {
	resource := p.Resource{Key: "claims", Title: "兑换记录", Endpoint: "/claims?mine=1", PollIntervalSeconds: 5, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("summary", "兑换内容", "text", false), c.Field("status", "状态", "status", false), c.Field("createdAt", "兑换时间", "datetime", false), c.Field("redeemedAt", "完成时间", "datetime", false), c.Field("lastError", "处理结果", "text", false)}}
	if admin {
		resource.Endpoint = "/claims"
		resource.Columns = append([]p.Field{c.Field("userId", "用户", "copy", false)}, resource.Columns...)
	}
	return resource
}

func (a *app) claimsView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权管理兑换记录")
	}
	return p.View{Title: "兑换记录", Resources: []p.Resource{claimResource(true)}}, nil
}

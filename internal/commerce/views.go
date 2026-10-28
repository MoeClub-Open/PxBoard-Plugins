package commerce

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

func planResource() p.Resource {
	fields := []p.Field{
		c.Field("name", "套餐名称", "text", true),
		{Key: "quotaGiB", Label: "每周期流量（GB）", Type: "number", Default: 100},
		c.Field("unlimited", "不限流量", "boolean", false),
		{Key: "groups", Label: "可用节点分组", Type: "groups", Required: true, Default: "default", OptionsEndpoint: "/api/v1/extensions/nodes/groups/options"},
		c.Select("kind", "套餐类型", "subscription|订阅 / 续费", "time|仅延长时间", "traffic|流量加油包", "reset|重置流量"),
		{Key: "periods", Label: "购买周期与价格", Type: "periods", Required: true},
		{Key: "enabled", Label: "上架", Type: "boolean", Default: true},
		{Key: "visible", Label: "公开展示", Type: "boolean", Default: true},
		{Key: "renewable", Label: "允许原套餐续费", Type: "boolean", Default: true},
		c.Field("reviewRequired", "需要核对", "boolean", false),
	}
	fields[4].Default = "subscription"
	return p.Resource{Key: "plans", Title: "套餐", Endpoint: "/plans", CreateLabel: "创建套餐", Editable: true, Deletable: true, Fields: fields, Columns: []p.Field{
		c.Field("name", "套餐名称", "text", false), c.Field("quotaGiB", "周期流量", "gib", false), c.Field("unlimited", "不限流量", "boolean", false), c.GroupField("groups", "节点分组", false), c.Field("kind", "类型", "tag", false), c.Field("reviewRequired", "待核对", "boolean", false), c.Field("enabled", "可供新购", "boolean", false), c.Field("visible", "公开展示", "boolean", false), c.Field("renewable", "可续费", "boolean", false),
	}}
}
func couponResource() p.Resource {
	f := []p.Field{c.Field("code", "优惠码", "text", true), c.Select("type", "优惠方式", "percent|减免百分比", "fixed|固定金额"), c.Field("discount", "优惠额度", "discount", true), c.Field("limit", "总次数（0为不限）", "number", true), c.Field("expiresAt", "到期时间", "datetime", false), c.Field("enabled", "启用", "boolean", false)}
	f[1].Default = "percent"
	f[2].Default = 10
	f[3].Default = 100
	f[5].Default = true
	return p.Resource{Key: "coupons", Title: "优惠码", Endpoint: "/coupons", CreateLabel: "创建优惠码", Editable: true, Deletable: true, Fields: f, Columns: []p.Field{c.Field("code", "优惠码", "copy", false), c.Field("type", "优惠方式", "tag", false), c.Field("discount", "优惠额度", "discount", false), c.Field("limit", "可用次数", "number", false), c.Field("expiresAt", "到期时间", "datetime", false), c.Field("enabled", "启用", "boolean", false)}}
}
func (a *app) checkoutAction() *p.Action {
	f := c.Field("channelId", "支付渠道", "select", true)
	f.OptionsEndpoint = "/api/v1/extensions/payments/channels"
	return &p.Action{Key: "checkout", Label: "在线支付", Endpoint: "/api/v1/extensions/payments/checkout", Bind: map[string]string{"orderId": "id"}, Fields: []p.Field{f}, When: map[string][]string{"status": {"pending"}}}
}
func (a *app) planView(r *http.Request) (p.View, error) {
	res := planResource()
	res.Actions = []p.Action{{Key: "details", Label: "套餐详情"}}
	if !p.Admin(r) {
		res.CreateLabel = ""
		res.Editable = false
		res.Deletable = false
		period := c.Field("periodId", "购买周期", "select", true)
		period.OptionsEndpoint = "/api/v1/extensions/commerce/plans/{id}/period-options"
		res.Actions = []p.Action{{Key: "buy", Label: "购买", Endpoint: "/orders", Bind: map[string]string{"planId": "id"}, Fields: []p.Field{period, c.Field("coupon", "优惠码", "text", false)}}}
	}
	return p.View{Title: "套餐", Resources: []p.Resource{res}}, nil
}
func (a *app) orderView(r *http.Request) (p.View, error) {
	f := c.Field("periodId", "套餐与周期", "select", true)
	f.OptionsEndpoint = "/api/v1/extensions/commerce/period-options"
	fields := []p.Field{f, c.Field("coupon", "优惠码", "text", false)}
	if p.Admin(r) {
		fields = append([]p.Field{c.UserField()}, fields...)
	}
	actions := []p.Action{{Key: "pay", Label: "余额支付", Confirm: "使用账户余额支付此订单？", When: map[string][]string{"status": {"pending", "paid"}, "kind": {"subscription", "time", "traffic", "reset"}}}, {Key: "cancel", Label: "取消", Confirm: "取消此订单？", When: map[string][]string{"status": {"pending"}}}}
	if checkout := a.checkoutAction(); checkout != nil {
		actions = append(actions, *checkout)
	}
	if p.Admin(r) {
		actions = append(actions, p.Action{Key: "confirm", Label: "确认收款", Confirm: "确认已在外部实际收到这笔款项？", When: map[string][]string{"status": {"pending", "paid"}}}, p.Action{Key: "refund", Label: "退回余额", Danger: true, Confirm: "撤销该订单的访问权益，并将金额退回账户余额？", When: map[string][]string{"status": {"fulfilled", "refunding"}, "kind": {"subscription", "time", "traffic", "reset"}}})
		actions[len(actions)-1].When["refundable"] = []string{"true"}
	}
	orders, e := a.Filter("orders", func(row p.Record) bool { return c.Own(r, row) })
	if e != nil {
		return p.View{}, e
	}
	total, pending := 0.0, 0
	for _, order := range orders {
		if p.Text(order, "status") == "fulfilled" {
			total += p.Number(order, "amount")
		}
		if p.Text(order, "status") == "pending" {
			pending++
		}
	}
	resource := p.Resource{Paginated: true, Key: "orders", Title: "订单记录", Endpoint: "/orders", CreateLabel: "创建订单", Fields: fields, Columns: []p.Field{c.Field("number", "订单号", "copy", false), c.Field("planName", "商品", "text", false), c.Field("amount", "实付金额", "money", false), c.Field("status", "状态", "status", false), c.Field("method", "支付方式", "tag", false), c.Field("createdAt", "创建时间", "datetime", false)}, Actions: actions}
	if p.Admin(r) {
		resource.Filters = orderFilterFields()
		resource.Settings = &p.ResourceSettings{Title: "订单设置", Endpoint: "/order-settings", Fields: []p.Field{c.Field("paymentTimeoutMinutes", "待支付有效期（分钟）", "number", true)}}
		resource.Columns = append(resource.Columns, c.Field("email", "用户", "text", false), c.Field("cancelReasonLabel", "取消原因", "text", false), c.Field("canceledAt", "取消时间", "datetime", false))
	}
	return p.View{Title: "订单", Metrics: []p.Metric{{Label: "累计成交", Value: total / 100, Unit: "元"}, {Label: "待支付订单", Value: pending}}, Resources: []p.Resource{resource}}, nil
}
func (a *app) walletView(r *http.Request) (p.View, error) {
	balance, e := a.balance(p.Subject(r).ID)
	if e != nil {
		return p.View{}, e
	}
	res := p.Resource{Paginated: true, Key: "ledger", Title: "余额明细", Endpoint: "/ledger", Columns: []p.Field{c.Field("userId", "用户 ID", "text", false), c.Field("amount", "变动金额", "money", false), c.Field("reason", "原因", "text", false), c.Field("createdAt", "时间", "datetime", false)}}
	if p.Admin(r) {
		res.CreateLabel = "调整余额"
		res.Fields = []p.Field{c.UserField(), c.Field("amount", "调整金额（扣款填负数）", "money", true), c.Field("reason", "调整原因", "text", true)}
	}
	actions := []p.Action{}
	if checkout := a.checkoutAction(); checkout != nil {
		actions = append(actions, *checkout)
	}
	return p.View{Title: "余额", Metrics: []p.Metric{{Label: "我的可用余额", Value: balance / 100, Unit: "元"}}, Resources: []p.Resource{res, {Key: "recharge", Title: "充值", Endpoint: "/orders?kind=recharge", CreateLabel: "余额充值", Fields: []p.Field{{Key: "kind", Label: "类型", Type: "hidden", Default: "recharge"}, c.Field("amount", "充值金额", "money", true)}, Columns: []p.Field{c.Field("number", "订单号", "text", false), c.Field("amount", "金额", "money", false), c.Field("status", "状态", "status", false)}, Actions: actions}}}, nil
}
func (a *app) couponView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, problemPermission()
	}
	resource := couponResource()
	families, periods, err := a.planOptions()
	if err != nil {
		return p.View{}, err
	}
	resource.Fields = append(resource.Fields, c.Field("startsAt", "开始时间", "datetime", false), c.Field("perUserLimit", "每账号次数（0为不限）", "number", false), p.Field{Key: "planFamilyIds", Label: "适用套餐", Type: "multiselect", Options: families}, p.Field{Key: "periodIds", Label: "适用购买周期", Type: "multiselect", Options: periods})
	return p.View{Title: "优惠码", Resources: []p.Resource{resource}}, nil
}
func problemPermission() error { return fmt.Errorf("仅管理员可管理此功能") }

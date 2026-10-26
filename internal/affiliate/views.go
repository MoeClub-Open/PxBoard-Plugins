package affiliate

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) referralView(r *http.Request) (p.View, error) {
	available, e := a.available(p.Subject(r).ID)
	if e != nil {
		return p.View{}, e
	}
	withdrawals := p.Resource{
		Key: "withdrawals", Title: "结算记录", Endpoint: "/withdrawals", CreateLabel: "结算佣金",
		Fields: []p.Field{
			c.Field("amount", "金额", "money", true),
			c.Select("method", "结算方式", "wallet|转入余额", "cash|现金提现"),
			{Key: "payoutAccount", Label: "收款账户", Type: "text", Required: true, When: map[string][]string{"method": {"cash"}}},
		},
		Columns: []p.Field{
			c.Field("userId", "用户 ID", "text", false), c.Field("method", "方式", "tag", false),
			c.Field("payoutAccount", "收款账户", "text", false), c.Field("amount", "金额", "money", false),
			c.Field("statusLabel", "状态", "tag", false), c.Field("reference", "打款流水", "copy", false),
			c.Field("createdAt", "创建时间", "datetime", false),
		},
	}
	withdrawals.Fields[1].Default = "wallet"
	if p.Admin(r) {
		withdrawals.CreateLabel = ""
		withdrawals.Actions = []p.Action{
			{Key: "paid", Label: "登记已打款", Confirm: "确认已经在外部完成打款？此操作只记录结果。", Fields: []p.Field{c.Field("reference", "打款流水号", "text", true)}, When: map[string][]string{"status": {"approved"}, "method": {"cash"}}},
			{Key: "approve", Label: "通过", Confirm: "审批此现金提现申请？通过后需要另行打款。", When: map[string][]string{"status": {"pending"}, "method": {"cash"}}},
			{Key: "reject", Label: "拒绝", Confirm: "拒绝此申请？", When: map[string][]string{"status": {"pending"}}},
		}
	} else {
		withdrawals.Actions = []p.Action{
			{Key: "transfer", Label: "转入余额", Confirm: "将这笔佣金转入账户余额？", When: map[string][]string{"status": {"pending"}, "method": {"wallet"}}},
			{Key: "cancel", Label: "取消", Confirm: "取消此结算？", When: map[string][]string{"status": {"pending"}}},
		}
	}
	return p.View{Title: "推广返佣", Metrics: []p.Metric{{Label: "我的可用佣金", Value: available / 100, Unit: "元"}}, Resources: []p.Resource{{Key: "commissions", Title: "佣金记录", Endpoint: "/commissions", Columns: []p.Field{c.Field("userId", "用户 ID", "text", false), c.Field("buyerId", "购买用户", "text", false), c.Field("orderId", "订单", "copy", false), c.Field("amount", "佣金", "money", false), c.Field("status", "状态", "status", false), c.Field("availableAt", "可用时间", "datetime", false), c.Field("createdAt", "产生时间", "datetime", false)}}, withdrawals}}, nil
}

func (a *app) settingsView(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可修改推广设置")
	}
	settings := []p.Field{c.Field("commissionPercent", "返佣比例（%）", "number", true), c.Field("commissionHoldDays", "佣金冻结（天）", "number", true), c.Field("commissionFirstPurchaseOnly", "仅首次购买返佣", "boolean", false)}
	profile := []p.Field{c.UserField(a.C), c.Field("commissionPercent", "个人比例（留空跟随系统）", "number", false), c.Select("commissionMode", "返佣方式", "system|跟随系统", "recurring|持续返佣", "firstPurchase|仅首次购买")}
	profile[2].Default = "system"
	return p.View{Title: "推广设置", Resources: []p.Resource{{Key: "settings", Singleton: true, Title: "推广规则", Endpoint: "/settings", Editable: true, Fields: settings, Columns: settings}, {Key: "profiles", Title: "个人规则", Endpoint: "/profiles", CreateLabel: "设置个人规则", Editable: true, Fields: profile, Columns: []p.Field{c.Field("userId", "用户 ID", "text", false), c.Field("commissionPercent", "个人比例（%）", "number", false), c.Field("commissionMode", "返佣方式", "tag", false), c.Field("hasPriorPurchase", "迁移前有购买记录", "boolean", false)}}}}, nil
}

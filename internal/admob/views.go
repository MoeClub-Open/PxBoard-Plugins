package admob

import (
	"fmt"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("无权管理激励广告")
	}
	fields := []p.Field{
		{Key: "planEnabled", Label: "开启套餐激励发放", Type: "boolean"},
		{Key: "planDurationDays", Label: "套餐赠送天数", Type: "number", Required: true, Default: 1},
		{Key: "planQuotaGiB", Label: "套餐赠送流量（GB）", Type: "number", Required: true, Default: 5},
		c.GroupField("planGroups", "套餐可用节点分组", true),
		{Key: "planRequiresInactive", Label: "仅当前无有效套餐时可领取", Type: "boolean", Default: true},
		{Key: "planDeviceLimit", Label: "套餐设备数限制（0为不限）", Type: "number", Default: 0},
		{Key: "planSpeedLimitMbps", Label: "套餐速率限制 Mbps（0为不限）", Type: "number", Default: 0},
		{Key: "planAdUnitId", Label: "套餐广告单元校验（选填）", Type: "text", Hint: "留空不限制；填写后校验回调广告单元 ID 或尾号"},
		{Key: "pointsEnabled", Label: "开启余额激励发放", Type: "boolean", Hint: "需同时启用商业钱包插件"},
		{Key: "pointsAmount", Label: "每次奖励余额", Type: "money", Default: 200},
		{Key: "pointsAdUnitId", Label: "余额广告单元校验（选填）", Type: "text", Hint: "留空不限制；填写后校验回调广告单元 ID 或尾号"},
		{Key: "tokenTtlSeconds", Label: "令牌有效期（秒）", Type: "number", Default: 900},
		{Key: "callbackMaxAgeSeconds", Label: "回调最大时间偏移（秒）", Type: "number", Default: 3600},
		{Key: "ssvSecret", Label: "SSV 签名密钥（留空自动生成）", Type: "text"},
		{Key: "callbackPath", Label: "SSV 回调地址", Type: "copy", ReadOnly: true},
	}
	columns := []p.Field{
		c.Field("planEnabled", "套餐激励", "boolean", false),
		c.Field("planDurationDays", "赠送天数", "number", false),
		c.Field("planQuotaGiB", "赠送流量（GB）", "number", false),
		c.Field("pointsEnabled", "余额激励", "boolean", false),
		c.Field("pointsAmount", "每次奖励余额", "money", false),
		{Key: "callbackPath", Label: "SSV 回调地址", Type: "copy", ReadOnly: true},
	}
	settingsRes := p.Resource{
		Key:       "settings",
		Title:     "发奖规则",
		Endpoint:  "/settings",
		Singleton: true,
		Editable:  true,
		Fields:    fields,
		Columns:   columns,
	}
	logsRes := p.Resource{
		Key:       "logs",
		Title:     "激励记录",
		Endpoint:  "/logs",
		Paginated: true,
		Empty:     "暂无激励广告回调记录",
		Columns: []p.Field{
			c.Field("userId", "用户", "copy", false),
			c.Field("scene", "场景", "text", false),
			c.Field("rewardContent", "奖励内容", "text", false),
			c.Field("status", "状态", "status", false),
			c.Field("transactionId", "交易号", "copy", false),
			c.Field("createdAt", "时间", "datetime", false),
			c.Field("error", "错误信息", "text", false),
		},
	}
	return p.View{
		Title:     "激励广告",
		Resources: []p.Resource{settingsRes, logsRes},
	}, nil
}

package commerce

import (
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func orderFilters(r *http.Request) ([]p.Record, error) {
	filters := c.OwnerFilters(r)
	for _, field := range []string{"status", "kind", "method", "cancelReason"} {
		if value := r.URL.Query().Get(field); value != "" {
			filters = append(filters, p.Record{"field": field, "value": value})
		}
	}
	if user := r.URL.Query().Get("userId"); p.Admin(r) && user != "" {
		filters = append(filters, p.Record{"field": "userId", "value": user})
	}
	for _, item := range []struct{ key, op string }{{"createdAtFrom", "gte"}, {"createdAtTo", "lte"}} {
		if raw := r.URL.Query().Get(item.key); raw != "" {
			value, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return nil, err
			}
			filters = append(filters, p.Record{"field": "createdAt", "op": item.op, "value": value.UTC().Format(time.RFC3339)})
		}
	}
	return filters, nil
}

func orderFilterFields() []p.Field {
	fields := []p.Field{
		c.Select("status", "订单状态", "pending|待支付", "paid|开通中", "fulfilled|已完成", "canceled|已取消", "credited|已折抵", "refunding|退款中", "refunded|已退款"),
		c.Select("kind", "订单类型", "subscription|套餐订阅", "recharge|余额充值", "time|延长期限", "traffic|流量加油包", "reset|重置流量"),
		c.Select("method", "支付方式", "wallet|余额", "online|在线支付", "manual|人工收款"),
		c.Select("cancelReason", "取消原因", "timeout|超时取消", "manual|手动取消"),
		c.UserField(),
		{Key: "createdAt", Label: "创建时间", Type: "daterange"},
	}
	for i := range fields {
		fields[i].Required = false
	}
	return fields
}

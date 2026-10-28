package commerce

import (
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func paymentTimeout(config p.Record) time.Duration {
	minutes := p.Number(config, "paymentTimeoutMinutes")
	if minutes <= 0 {
		minutes = 30
	}
	return time.Duration(minutes) * time.Minute
}

func orderDeadline(row p.Record, timeout time.Duration) time.Time {
	created, err := time.Parse(time.RFC3339, p.Text(row, "createdAt"))
	if err != nil {
		return time.Time{}
	}
	return created.Add(timeout)
}

func orderExpired(row p.Record, timeout time.Duration, now time.Time) bool {
	deadline := orderDeadline(row, timeout)
	return p.Text(row, "status") == "pending" && !p.Bool(row, "imported") && !deadline.IsZero() && !now.Before(deadline)
}

func annotateOrder(row p.Record, timeout time.Duration) {
	if p.Text(row, "status") == "pending" {
		if deadline := orderDeadline(row, timeout); !deadline.IsZero() {
			row["expiresAt"] = deadline.UTC().Format(time.RFC3339)
		}
	}
	row["cancelReasonLabel"] = map[string]string{"timeout": "超时取消", "manual": "手动取消"}[p.Text(row, "cancelReason")]
}

func (a *app) cancelOrder(row p.Record, reason string, now time.Time) error {
	row["status"], row["cancelReason"] = "canceled", reason
	row["canceledAt"], row["updatedAt"] = now.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339)
	return a.C.Put("orders", row)
}

func (a *app) expirePending(row p.Record, timeout time.Duration, now time.Time) (bool, error) {
	if !orderExpired(row, timeout, now) {
		return false, nil
	}
	debited, err := a.walletDebited(row)
	if err != nil || debited {
		return false, err
	}
	return true, a.cancelOrder(row, "timeout", now)
}

func (a *app) orderSettings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	config, err := a.config()
	if c.Err(w, err) {
		return
	}
	if r.Method == "PATCH" {
		var body struct{ PaymentTimeoutMinutes int }
		if c.Err(w, p.Decode(r, &body)) {
			return
		}
		if body.PaymentTimeoutMinutes < 1 {
			p.Fail(w, 400, "待支付订单有效期须为正整数分钟")
			return
		}
		config["paymentTimeoutMinutes"] = body.PaymentTimeoutMinutes
		if c.Err(w, a.C.Put("settings", config)) {
			return
		}
	}
	p.JSON(w, p.Record{"paymentTimeoutMinutes": int(paymentTimeout(config) / time.Minute)})
}

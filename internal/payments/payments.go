package payments

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /channels", a.channels)
	a.Mux.HandleFunc("POST /channels", a.save)
	a.Mux.HandleFunc("PATCH /channels/{id}", a.save)
	a.Mux.HandleFunc("DELETE /channels/{id}", a.deleteChannel)
	a.Mux.HandleFunc("POST /checkout", a.checkout)
	a.Mux.HandleFunc("POST /webhooks/{id}", a.webhook)
	a.Mux.HandleFunc("GET /webhooks/{id}", a.webhook)
	a.Mux.HandleFunc("GET /transactions", a.transactions)
	a.Mux.HandleFunc("POST /transactions/{id}/resolve", a.resolve)
	a.Events(a.event)
	a.View("payments", a.view)
	return a
}
func (a *app) transactions(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("transactions")
	if !c.Err(w, e) {
		labels := map[string]string{"received": "等待处理", "queued": "已交给订单处理", "needs_review": "需要人工核对", "resolved": "已核对"}
		for _, row := range rows {
			row["statusLabel"] = labels[p.Text(row, "status")]
		}
		p.JSON(w, rows)
	}
}

func (a *app) resolve(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) || c.Err(w, c.Validate(b, "note")) {
		return
	}
	row, e := a.Find("transactions", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if p.Text(row, "status") != "needs_review" {
		p.Fail(w, 400, "只有需要人工核对的支付记录可以登记结果")
		return
	}
	row["status"], row["resolution"] = "resolved", b["note"]
	row["resolvedAt"], row["resolvedBy"] = c.Now(), p.Subject(r).Email
	if !c.Err(w, a.C.Put("transactions", row)) {
		p.JSON(w, true)
	}
}
func (a *app) view(r *http.Request) (p.View, error) {
	return p.View{Title: "支付渠道", Resources: []p.Resource{{Key: "channels", Title: "支付渠道", Endpoint: "/channels", CreateLabel: "添加支付渠道", Editable: true, Deletable: true, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("provider", "渠道", "tag", false), c.Field("enabled", "启用", "boolean", false), c.Field("webhookUrl", "回调地址", "link", false)}, Fields: []p.Field{c.Field("name", "渠道名称", "text", true), c.Select("provider", "渠道类型", "stripe|Stripe", "epay|易支付"), c.Field("apiKey", "支付密钥（修改时留空保留）", "password", false), c.Field("webhookSecret", "Stripe 回调签名密钥", "password", false), c.Field("merchantId", "易支付商户号", "text", false), c.Field("gatewayUrl", "易支付网关地址", "url", false), c.Field("publicUrl", "面板公开地址", "url", true), c.Field("enabled", "启用", "boolean", false)}}, {Key: "transactions", Title: "支付记录", Endpoint: "/transactions", Columns: []p.Field{c.Field("orderId", "订单 ID", "text", false), c.Field("amount", "金额", "money", false), c.Field("currency", "币种", "tag", false), c.Field("transactionId", "支付流水号", "copy", false), c.Field("statusLabel", "处理状态", "status", false), c.Field("message", "核对原因", "text", false), c.Field("resolution", "核对结果", "text", false), c.Field("createdAt", "时间", "datetime", false)}, Actions: []p.Action{{Key: "resolve", Label: "登记核对结果", Fields: []p.Field{c.Field("note", "处理结果", "text", true)}, When: map[string][]string{"status": {"needs_review"}}}}}}}, nil
}

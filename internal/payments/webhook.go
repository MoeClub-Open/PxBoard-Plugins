package payments

import (
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) webhook(w http.ResponseWriter, r *http.Request) {
	channel, e := a.Find("channels", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	var data p.Record
	switch p.Text(channel, "provider") {
	case "stripe":
		if r.Method != "POST" {
			p.Fail(w, 405, "Stripe 回调需要使用 POST")
			return
		}
		raw, e := io.ReadAll(r.Body)
		if c.Err(w, e) {
			return
		}
		if !verifyStripe(raw, r.Header.Get("Stripe-Signature"), p.Text(channel, "webhookSecret")) {
			p.Fail(w, 400, "支付签名无效")
			return
		}
		var event struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Data struct {
				Object struct {
					ID            string `json:"id"`
					PaymentIntent string `json:"payment_intent"`
					Reference     string `json:"client_reference_id"`
					Amount        int64  `json:"amount_total"`
					Currency      string `json:"currency"`
					PaymentStatus string `json:"payment_status"`
				} `json:"object"`
			} `json:"data"`
		}
		if c.Err(w, json.Unmarshal(raw, &event)) {
			return
		}
		if (event.Type != "checkout.session.completed" && event.Type != "checkout.session.async_payment_succeeded") || event.Data.Object.PaymentStatus != "paid" {
			p.JSON(w, true)
			return
		}
		o := event.Data.Object
		transaction := o.PaymentIntent
		if transaction == "" {
			transaction = o.ID
		}
		data = p.Record{"orderId": o.Reference, "amount": o.Amount, "currency": strings.ToUpper(o.Currency), "transactionId": transaction, "providerEventId": event.ID}
	case "epay":
		if e = r.ParseForm(); c.Err(w, e) {
			return
		}
		if !hmac.Equal([]byte(strings.ToLower(r.Form.Get("sign"))), []byte(epaySign(r.Form, p.Text(channel, "apiKey")))) || r.Form.Get("pid") != p.Text(channel, "merchantId") || r.Form.Get("trade_status") != "TRADE_SUCCESS" {
			p.Fail(w, 400, "支付签名或状态无效")
			return
		}
		amount, e := minor(r.Form.Get("money"))
		if c.Err(w, e) {
			return
		}
		data = p.Record{"orderId": r.Form.Get("out_trade_no"), "amount": amount, "currency": "CNY", "transactionId": r.Form.Get("trade_no")}
	default:
		p.Fail(w, 400, "支付渠道类型无效")
		return
	}
	if p.Text(data, "orderId") == "" || p.Text(data, "transactionId") == "" || p.Number(data, "amount") <= 0 {
		p.Fail(w, 400, "支付流水缺少订单、金额或交易编号")
		return
	}
	data["id"] = p.Text(channel, "id") + ":" + p.Text(data, "transactionId")
	data["channelId"] = channel["id"]
	data["provider"] = channel["provider"]
	if c.Err(w, a.recordPayment(data)) {
		return
	}
	if p.Text(channel, "provider") == "epay" {
		_, _ = w.Write([]byte("success"))
	} else {
		p.JSON(w, true)
	}
}

func (a *app) recordPayment(data p.Record) error {
	rows, e := a.C.List("transactions")
	if e != nil {
		return e
	}
	for _, old := range rows {
		if old["id"] != data["id"] {
			continue
		}
		if old["orderId"] != data["orderId"] || p.Number(old, "amount") != p.Number(data, "amount") || old["currency"] != data["currency"] {
			return fmt.Errorf("同一支付流水的订单或金额发生变化，请人工核对")
		}
		if status := p.Text(old, "status"); status == "queued" || status == "needs_review" || status == "resolved" {
			return nil
		}
		data = old
		break
	}
	if data["createdAt"] == nil {
		data["createdAt"] = c.Now()
	}
	data["status"] = "received"
	if e = a.C.Put("transactions", data); e != nil {
		return e
	}
	var order p.Record
	if e = a.C.Call("commerce", "GET", "/internal/orders/"+url.PathEscape(p.Text(data, "orderId")), nil, &order); e != nil {
		return e
	}
	reason := ""
	if p.Number(order, "amount") != p.Number(data, "amount") || order["currency"] != data["currency"] {
		reason = "支付金额或币种与订单不符"
	} else if status := p.Text(order, "status"); status != "pending" && !(status == "paid" && p.Text(order, "method") == "online" && order["paymentId"] == data["id"]) {
		if status == "fulfilled" && p.Text(order, "method") == "online" && order["paymentId"] == data["id"] {
			data["status"] = "queued"
			return a.C.Put("transactions", data)
		}
		reason = "订单已关闭或已由其他款项支付"
	}
	if reason != "" {
		data["status"], data["message"] = "needs_review", reason
		return a.C.Put("transactions", data)
	}
	if e = a.C.Emit("payment.succeeded", data); e != nil {
		return e
	}
	data["status"] = "queued"
	return a.C.Put("transactions", data)
}

func (a *app) event(e p.Event) error {
	if e.Type != "payment.review-required" {
		return nil
	}
	if e.Source != "commerce" {
		return fmt.Errorf("支付核对事件来源无效")
	}
	row, err := a.Find("transactions", p.Text(e.Data, "paymentId"))
	if err != nil {
		return err
	}
	row["status"], row["message"] = "needs_review", e.Data["reason"]
	return a.C.Put("transactions", row)
}

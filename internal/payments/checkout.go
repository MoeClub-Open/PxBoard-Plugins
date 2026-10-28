package payments

import (
	"encoding/json"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (a *app) checkout(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	var order p.Record
	if c.Err(w, a.C.Call("commerce", "GET", "/internal/orders/"+url.PathEscape(p.Text(b, "orderId")), nil, &order)) {
		return
	}
	if !c.Own(r, order) {
		p.Fail(w, 403, "无权支付此订单")
		return
	}
	if p.Text(order, "status") != "pending" || p.Bool(order, "imported") {
		p.Fail(w, 400, "订单已支付或已关闭")
		return
	}
	channel, e := a.Find("channels", p.Text(b, "channelId"))
	if c.Err(w, e) {
		return
	}
	if !p.Bool(channel, "enabled") {
		p.Fail(w, 400, "支付渠道已停用")
		return
	}
	if p.Number(order, "amount") <= 0 {
		p.Fail(w, 400, "此订单无需在线付款，请直接使用余额支付")
		return
	}
	if c.Err(w, a.C.Put("checkouts", p.Record{"id": p.Text(channel, "id") + ":" + p.Text(order, "id"), "channelId": channel["id"], "orderId": order["id"], "createdAt": c.Now()})) {
		return
	}
	base := strings.TrimSuffix(p.Text(channel, "publicUrl"), "/")
	var redirect string
	switch p.Text(channel, "provider") {
	case "stripe":
		form := url.Values{"mode": {"payment"}, "success_url": {base + "/user/orders?payment=success"}, "cancel_url": {base + "/user/orders?payment=canceled"}, "client_reference_id": {p.Text(order, "id")}, "metadata[orderId]": {p.Text(order, "id")}, "line_items[0][price_data][currency]": {"cny"}, "line_items[0][price_data][unit_amount]": {strconv.Itoa(int(p.Number(order, "amount")))}, "line_items[0][price_data][product_data][name]": {p.Text(order, "planName")}, "line_items[0][quantity]": {"1"}}
		req, _ := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(form.Encode()))
		req.SetBasicAuth(p.Text(channel, "apiKey"), "")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Idempotency-Key", p.Text(order, "id"))
		res, e := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if c.Err(w, e) {
			return
		}
		defer res.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		if c.Err(w, e) {
			return
		}
		if res.StatusCode >= 400 {
			p.Fail(w, 502, "Stripe 创建支付失败，请检查渠道配置")
			return
		}
		var result struct {
			URL string `json:"url"`
		}
		if c.Err(w, json.Unmarshal(raw, &result)) {
			return
		}
		redirect = result.URL
	case "epay":
		paymentType := p.Text(channel, "paymentType")
		if paymentType == "" {
			paymentType = "alipay"
		}
		form := url.Values{"pid": {p.Text(channel, "merchantId")}, "type": {paymentType}, "out_trade_no": {p.Text(order, "id")}, "notify_url": {base + "/api/v1/extensions/payments/webhooks/" + p.Text(channel, "id")}, "return_url": {base + "/user/orders"}, "name": {p.Text(order, "planName")}, "money": {fmt.Sprintf("%.2f", p.Number(order, "amount")/100)}}
		form.Set("sign", epaySign(form, p.Text(channel, "apiKey")))
		form.Set("sign_type", "MD5")
		redirect = strings.TrimSuffix(p.Text(channel, "gatewayUrl"), "/") + "/submit.php?" + form.Encode()
	}
	if redirect == "" {
		p.Fail(w, 502, "支付渠道没有返回收银台地址")
		return
	}
	p.JSON(w, p.Record{"redirectUrl": redirect})
}

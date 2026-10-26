package payments

import (
	"bytes"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	a.Mux.HandleFunc("GET /channels", a.channels)
	a.Mux.HandleFunc("POST /channels", a.save)
	a.Mux.HandleFunc("PATCH /channels/{id}", a.save)
	a.Mux.HandleFunc("DELETE /channels/{id}", func(w http.ResponseWriter, r *http.Request) {
		if c.Require(w, r) {
			if !c.Err(w, a.C.Delete("channels", r.PathValue("id"))) {
				p.JSON(w, true)
			}
		}
	})
	a.Mux.HandleFunc("POST /checkout", a.checkout)
	a.Mux.HandleFunc("POST /webhooks/{id}", a.webhook)
	a.Mux.HandleFunc("GET /webhooks/{id}", a.webhook)
	a.Mux.HandleFunc("GET /transactions", a.transactions)
	a.View("payments", a.view)
	return a
}
func (a *app) channels(w http.ResponseWriter, r *http.Request) {
	if !p.Service(r, "commerce") && !c.Member(w, r) {
		return
	}
	rows, e := a.C.List("channels")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	for _, v := range rows {
		if p.Admin(r) {
			delete(v, "apiKey")
			delete(v, "webhookSecret")
			v["webhookUrl"] = "/api/v1/extensions/payments/webhooks/" + p.Text(v, "id")
			out = append(out, v)
		} else if p.Bool(v, "enabled") {
			out = append(out, p.Record{"id": v["id"], "name": v["name"], "provider": v["provider"]})
		}
	}
	p.JSON(w, out)
}
func (a *app) save(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if r.Method == "PATCH" {
		old, e := a.Find("channels", r.PathValue("id"))
		if c.Err(w, e) {
			return
		}
		for k, v := range b {
			if (k == "apiKey" || k == "webhookSecret") && v == "" {
				continue
			}
			old[k] = v
		}
		b = old
	} else {
		delete(b, "id")
	}
	c.Defaults(b, nil, p.Record{"enabled": true, "provider": "stripe"})
	if c.Err(w, c.Validate(b, "name", "provider", "apiKey", "publicUrl")) {
		return
	}
	u, e := url.Parse(p.Text(b, "publicUrl"))
	if e != nil || u.Scheme != "https" || u.Host == "" {
		p.Fail(w, 400, "公开访问地址必须使用 HTTPS")
		return
	}
	if !c.Allowed(p.Text(b, "provider"), "stripe", "epay") {
		p.Fail(w, 400, "支付渠道无效")
		return
	}
	if p.Text(b, "provider") == "stripe" && p.Text(b, "webhookSecret") == "" {
		p.Fail(w, 400, "请填写 Stripe 回调签名密钥")
		return
	}
	if p.Text(b, "provider") == "epay" {
		g, e := url.Parse(p.Text(b, "gatewayUrl"))
		if e != nil || g.Scheme != "https" || g.Host == "" || p.Text(b, "merchantId") == "" {
			p.Fail(w, 400, "请填写 HTTPS 支付网关与商户号")
			return
		}
	}
	if !c.Err(w, a.C.Put("channels", b)) {
		p.JSON(w, p.Record{"id": b["id"]})
	}
}
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
	if p.Text(order, "status") != "pending" {
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
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
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
		form := url.Values{"pid": {p.Text(channel, "merchantId")}, "type": {"alipay"}, "out_trade_no": {p.Text(order, "id")}, "notify_url": {base + "/api/v1/extensions/payments/webhooks/" + p.Text(channel, "id")}, "return_url": {base + "/user/orders"}, "name": {p.Text(order, "planName")}, "money": {fmt.Sprintf("%.2f", p.Number(order, "amount")/100)}}
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
func (a *app) webhook(w http.ResponseWriter, r *http.Request) {
	channel, e := a.Find("channels", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	var data p.Record
	if p.Text(channel, "provider") == "stripe" {
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
					Reference     string  `json:"client_reference_id"`
					Amount        float64 `json:"amount_total"`
					Currency      string  `json:"currency"`
					PaymentStatus string  `json:"payment_status"`
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
		data = p.Record{"id": event.ID, "orderId": o.Reference, "amount": o.Amount, "currency": strings.ToUpper(o.Currency), "transactionId": event.ID}
	} else {
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
		data = p.Record{"id": "epay-" + r.Form.Get("trade_no"), "orderId": r.Form.Get("out_trade_no"), "amount": float64(amount), "currency": "CNY", "transactionId": r.Form.Get("trade_no")}
	}
	var order p.Record
	if c.Err(w, a.C.Call("commerce", "GET", "/internal/orders/"+url.PathEscape(p.Text(data, "orderId")), nil, &order)) {
		return
	}
	if p.Number(order, "amount") != p.Number(data, "amount") || order["currency"] != data["currency"] {
		p.Fail(w, 400, "支付金额或币种不符")
		return
	}
	data["channelId"] = channel["id"]
	data["createdAt"] = c.Now()
	if c.Err(w, a.C.Emit("payment.succeeded", data)) {
		return
	}
	if c.Err(w, a.C.Put("transactions", data)) {
		return
	}
	if p.Text(channel, "provider") == "epay" {
		_, _ = w.Write([]byte("success"))
	} else {
		p.JSON(w, true)
	}
}
func verifyStripe(raw []byte, header, key string) bool {
	var timestamp string
	sigs := []string{}
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		if kv[0] == "t" {
			timestamp = kv[1]
		} else if kv[0] == "v1" {
			sigs = append(sigs, kv[1])
		}
	}
	n, e := strconv.ParseInt(timestamp, 10, 64)
	if e != nil || time.Since(time.Unix(n, 0)).Abs() > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write(append([]byte(timestamp+"."), raw...))
	for _, sig := range sigs {
		b, e := hex.DecodeString(sig)
		if e == nil && hmac.Equal(b, mac.Sum(nil)) {
			return true
		}
	}
	return false
}
func epaySign(v url.Values, key string) string {
	keys := []string{}
	for k := range v {
		if k != "sign" && k != "sign_type" && v.Get(k) != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b bytes.Buffer
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k + "=" + v.Get(k))
	}
	b.WriteString(key)
	sum := md5.Sum(b.Bytes())
	return hex.EncodeToString(sum[:])
}
func minor(value string) (int64, error) {
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("金额无效")
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil {
		return 0, e
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("金额精度无效")
		}
		fraction = (parts[1] + "00")[:2]
	}
	n, e := strconv.ParseInt(fraction, 10, 64)
	return whole*100 + n, e
}
func (a *app) transactions(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	rows, e := a.C.List("transactions")
	if !c.Err(w, e) {
		p.JSON(w, rows)
	}
}
func (a *app) view(r *http.Request) (p.View, error) {
	return p.View{Title: "支付渠道", Resources: []p.Resource{{Key: "channels", Title: "支付渠道", Endpoint: "/channels", CreateLabel: "添加支付渠道", Editable: true, Deletable: true, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("provider", "渠道", "tag", false), c.Field("enabled", "启用", "boolean", false), c.Field("webhookUrl", "回调地址", "link", false)}, Fields: []p.Field{c.Field("name", "渠道名称", "text", true), c.Select("provider", "渠道类型", "stripe|Stripe", "epay|易支付"), c.Field("apiKey", "支付密钥（修改时留空保留）", "password", false), c.Field("webhookSecret", "Stripe 回调签名密钥", "password", false), c.Field("merchantId", "易支付商户号", "text", false), c.Field("gatewayUrl", "易支付网关地址", "url", false), c.Field("publicUrl", "面板公开地址", "url", true), c.Field("enabled", "启用", "boolean", false)}}, {Key: "transactions", Title: "支付记录", Endpoint: "/transactions", Columns: []p.Field{c.Field("orderId", "订单 ID", "text", false), c.Field("amount", "金额", "money", false), c.Field("currency", "币种", "tag", false), c.Field("transactionId", "支付流水号", "copy", false), c.Field("createdAt", "时间", "datetime", false)}}}}, nil
}

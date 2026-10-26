package payments

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"net/url"
	"strings"
)

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
			v["webhookUrl"] = strings.TrimRight(p.Text(v, "publicUrl"), "/") + "/api/v1/extensions/payments/webhooks/" + p.Text(v, "id")
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
		if provider := p.Text(b, "provider"); provider != "" && provider != p.Text(old, "provider") {
			used, e := a.channelUsed(r.PathValue("id"))
			if c.Err(w, e) {
				return
			}
			if used {
				p.Fail(w, 400, "此渠道已用于支付，请新建另一种类型的支付渠道")
				return
			}
		}
		for k, v := range b {
			if k == "id" {
				continue
			}
			if (k == "apiKey" || k == "webhookSecret") && v == "" {
				continue
			}
			old[k] = v
		}
		b = old
		b["id"] = r.PathValue("id")
	} else {
		delete(b, "id")
	}
	c.Defaults(b, nil, p.Record{"enabled": true, "provider": "stripe"})
	if c.Err(w, c.Validate(b, "name", "provider", "apiKey", "publicUrl")) {
		return
	}
	u, e := url.Parse(p.Text(b, "publicUrl"))
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || strings.Trim(u.Path, "/") != "" || u.RawQuery != "" || u.Fragment != "" {
		p.Fail(w, 400, "请填写面板的 HTTPS 根地址")
		return
	}
	b["publicUrl"] = strings.TrimRight(u.String(), "/")
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
		if e != nil || g.Scheme != "https" || g.Host == "" || g.User != nil || g.RawQuery != "" || g.Fragment != "" || p.Text(b, "merchantId") == "" {
			p.Fail(w, 400, "请填写 HTTPS 支付网关与商户号")
			return
		}
		b["gatewayUrl"] = strings.TrimRight(g.String(), "/")
	}
	if !c.Err(w, a.C.Put("channels", b)) {
		p.JSON(w, p.Record{"id": b["id"]})
	}
}

func (a *app) deleteChannel(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	used, e := a.channelUsed(r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if used {
		p.Fail(w, 400, "此渠道已用于支付，请停用以保留后续支付回调")
		return
	}
	if !c.Err(w, a.C.Delete("channels", r.PathValue("id"))) {
		p.JSON(w, true)
	}
}

func (a *app) channelUsed(id string) (bool, error) {
	for _, collection := range []string{"checkouts", "transactions"} {
		rows, e := a.C.List(collection)
		if e != nil {
			return false, e
		}
		for _, row := range rows {
			if p.Text(row, "channelId") == id {
				return true, nil
			}
		}
	}
	return false, nil
}

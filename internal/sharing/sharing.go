package sharing

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

type app struct{ *c.App }

func New() http.Handler {
	a := &app{c.New()}
	res := p.Resource{Key: "invites", Endpoint: "/invites", CreateLabel: "创建邀请", Editable: true, Deletable: true}
	a.CRUD(res, func(r *http.Request, b, old p.Record) error {
		c.Defaults(b, old, p.Record{"code": p.ID(), "limit": 1.0, "used": 0.0, "days": 30.0, "quotaGiB": 20.0, "groups": "default", "enabled": true})
		if p.Number(b, "limit") < 1 || p.Number(b, "days") < 1 || p.Number(b, "quotaGiB") < 0 {
			return fmt.Errorf("邀请次数、天数或流量无效")
		}
		if old != nil {
			b["used"] = old["used"]
			b["code"] = old["code"]
		}
		return nil
	})
	a.View("sharing", a.view)
	a.Mux.HandleFunc("POST /redeem", a.redeem)
	a.Mux.HandleFunc("POST /invites/{id}/claim", a.claim)
	return a
}
func (a *app) redeem(w http.ResponseWriter, r *http.Request) {
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	rows, e := a.C.List("invites")
	if c.Err(w, e) {
		return
	}
	var invite p.Record
	for _, row := range rows {
		if row["code"] == b["code"] {
			invite = row
		}
	}
	if invite == nil || !p.Bool(invite, "enabled") || p.Number(invite, "used") >= p.Number(invite, "limit") || p.Text(invite, "expiresAt") != "" && p.Text(invite, "expiresAt") < c.Now() {
		p.Fail(w, 400, "邀请无效、已到期或已用完")
		return
	}
	var created p.Record
	b["role"] = "member"
	if c.Err(w, a.C.Request("POST", "/api/v1/runtime/users", b, &created)) {
		return
	}
	if c.Err(w, a.redeemFor(invite, p.Text(created, "id"))) {
		return
	}
	p.JSON(w, p.Record{"email": b["email"], "message": "账户已创建，可以登录"})
}
func (a *app) claim(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	rows, e := a.C.List("invites")
	if c.Err(w, e) {
		return
	}
	for _, row := range rows {
		if row["code"] == b["code"] {
			if !c.Err(w, a.redeemFor(row, p.Subject(r).ID)) {
				p.JSON(w, true)
			}
			return
		}
	}
	p.Fail(w, 400, "邀请无效")
}
func (a *app) redeemFor(invite p.Record, user string) error {
	claims, e := a.C.List("claims")
	if e != nil {
		return e
	}
	for _, v := range claims {
		if v["userId"] == user && v["inviteId"] == invite["id"] {
			return nil
		}
	}
	if !p.Bool(invite, "enabled") || p.Number(invite, "used") >= p.Number(invite, "limit") || p.Text(invite, "expiresAt") != "" && p.Text(invite, "expiresAt") < c.Now() {
		return fmt.Errorf("邀请无效、已到期或已用完")
	}
	grant := p.Record{"userId": user, "grantId": "invite-" + p.Text(invite, "id") + "-" + user, "durationUnit": "day", "durationValue": invite["days"], "quotaGiB": invite["quotaGiB"], "groups": invite["groups"]}
	if e = a.C.Call("access", "POST", "/internal/grant", grant, nil); e != nil {
		return e
	}
	if e = a.C.Put("claims", p.Record{"id": p.Text(invite, "id") + "-" + user, "inviteId": invite["id"], "userId": user, "createdAt": c.Now()}); e != nil {
		return e
	}
	invite["used"] = p.Number(invite, "used") + 1
	return a.C.Put("invites", invite)
}
func (a *app) view(r *http.Request) (p.View, error) {
	if !p.Admin(r) {
		return p.View{}, fmt.Errorf("仅管理员可管理分享")
	}
	f := []p.Field{c.Field("name", "邀请名称", "text", true), c.Field("limit", "可使用次数", "number", true), c.Field("days", "可使用天数", "number", true), c.Field("quotaGiB", "每人流量（GB）", "number", true), c.Field("groups", "节点分组", "text", true), c.Field("expiresAt", "邀请到期时间", "datetime", false), c.Field("enabled", "启用", "boolean", false)}
	f[1].Default = 1
	f[2].Default = 30
	f[3].Default = 20
	f[4].Default = "default"
	f[6].Default = true
	return p.View{Title: "分享与邀请", Resources: []p.Resource{{Key: "invites", Title: "邀请", Endpoint: "/invites", CreateLabel: "创建邀请", Editable: true, Deletable: true, Columns: []p.Field{c.Field("name", "名称", "text", false), c.Field("code", "邀请码", "copy", false), c.Field("used", "已使用", "number", false), c.Field("limit", "可用次数", "number", false), c.Field("days", "天数", "number", false), c.Field("quotaGiB", "每人流量", "gib", false), c.Field("enabled", "启用", "boolean", false)}, Fields: f}}}, nil
}

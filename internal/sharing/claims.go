package sharing

import (
	"fmt"
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
	"sort"
	"strings"
)

func (a *app) findInvite(code string) (p.Record, error) {
	rows, e := a.C.List("invites")
	if e != nil {
		return nil, e
	}
	for _, row := range rows {
		if p.Text(row, "code") == strings.TrimSpace(code) {
			return row, nil
		}
	}
	return nil, fmt.Errorf("邀请无效")
}

func claimCount(claims []p.Record, invite string) float64 {
	number := 0.0
	for _, claim := range claims {
		if p.Text(claim, "inviteId") == invite {
			number++
		}
	}
	return number
}

func available(invite p.Record, claims []p.Record) bool {
	return invite != nil && p.Bool(invite, "enabled") && claimCount(claims, p.Text(invite, "id")) < p.Number(invite, "limit") && (p.Text(invite, "expiresAt") == "" || p.Text(invite, "expiresAt") >= c.Now())
}

func (a *app) redeem(w http.ResponseWriter, r *http.Request) {
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	invite, e := a.findInvite(p.Text(b, "code"))
	if c.Err(w, e) {
		return
	}
	claims, e := a.C.List("claims")
	if c.Err(w, e) {
		return
	}
	if !available(invite, claims) {
		p.Fail(w, 400, "邀请无效、已到期或已用完")
		return
	}
	var created p.Record
	b["role"] = "member"
	if c.Err(w, a.C.Request("POST", "/api/v1/runtime/users", b, &created)) {
		return
	}
	if e = a.redeemFor(invite, p.Text(created, "id")); e != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		p.JSON(w, p.Record{"email": b["email"], "pending": true, "message": "账户已创建，可以登录；访问权限开通中，也可在领取分享页重试"})
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
	invite, e := a.findInvite(p.Text(b, "code"))
	if c.Err(w, e) || c.Err(w, a.redeemFor(invite, p.Subject(r).ID)) {
		return
	}
	p.JSON(w, p.Record{"message": "分享已领取"})
}

func (a *app) redeemFor(invite p.Record, user string) error {
	claims, e := a.C.List("claims")
	if e != nil {
		return e
	}
	for _, claim := range claims {
		if p.Text(claim, "userId") == user && claim["inviteId"] == invite["id"] {
			if p.Text(claim, "status") != "pending" {
				return nil
			}
			return a.completeClaim(claim, claims)
		}
	}
	if !available(invite, claims) {
		return fmt.Errorf("邀请无效、已到期或已用完")
	}
	claim := p.Record{
		"id": p.Text(invite, "id") + "-" + user, "inviteId": invite["id"], "userId": user, "status": "pending", "createdAt": c.Now(),
		"grant": p.Record{"userId": user, "grantId": "invite-" + p.Text(invite, "id") + "-" + user, "durationUnit": "day", "durationValue": invite["days"], "quotaGiB": invite["quotaGiB"], "groups": invite["groups"]},
	}
	if e = a.C.Put("claims", claim); e != nil {
		return e
	}
	return a.completeClaim(claim, append(claims, claim))
}

func (a *app) completeClaim(claim p.Record, claims []p.Record) error {
	claim["attemptedAt"] = c.Now()
	if e := a.C.Put("claims", claim); e != nil {
		return e
	}
	invites, e := a.C.List("invites")
	if e != nil {
		return e
	}
	for _, invite := range invites {
		if invite["id"] == claim["inviteId"] {
			invite["used"] = claimCount(claims, p.Text(invite, "id"))
			if e = a.C.Put("invites", invite); e != nil {
				return e
			}
			break
		}
	}
	if e = a.C.Call("access", "POST", "/internal/grant", claim["grant"], nil); e != nil {
		return e
	}
	claim["status"] = "granted"
	return a.C.Put("claims", claim)
}

func (a *app) event(event p.Event) error {
	if event.Type != "system.tick" {
		return nil
	}
	if event.Source != "core" {
		return fmt.Errorf("定时事件来源无效")
	}
	claims, e := a.C.List("claims")
	if e != nil {
		return e
	}
	pending := []p.Record{}
	for _, claim := range claims {
		if p.Text(claim, "status") == "pending" {
			pending = append(pending, claim)
		}
	}
	sort.Slice(pending, func(i, j int) bool { return p.Text(pending[i], "attemptedAt") < p.Text(pending[j], "attemptedAt") })
	if len(pending) > 0 {
		return a.completeClaim(pending[0], claims)
	}
	return nil
}

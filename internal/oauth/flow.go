package oauth

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"golang.org/x/oauth2"
)

func (a *app) authorize(w http.ResponseWriter, r *http.Request) {
	var b struct{ ProviderID, Purpose, Proof, RedirectPath, Referral string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if b.Purpose != "login" && b.Purpose != "bind" || len(b.Proof) < 32 {
		p.Fail(w, 400, "授权请求无效，请重新发起")
		return
	}
	if b.Purpose == "bind" && !c.Member(w, r) {
		return
	}
	provider, e := a.Find("providers", b.ProviderID)
	if e != nil || !p.Bool(provider, "enabled") {
		p.Fail(w, 400, "此登录服务未启用")
		return
	}
	if c.Err(w, a.cleanup("flows")) || c.Err(w, a.cleanup("pendingBindings")) || c.Err(w, a.cleanup("pendingRegistrations")) {
		return
	}
	state, verifier := p.ID()+p.ID(), oauth2.GenerateVerifier()
	flow := p.Record{"id": state, "providerId": b.ProviderID, "purpose": b.Purpose, "proofDigest": digest(b.Proof), "verifier": verifier, "redirectPath": localPath(b.RedirectPath), "expiresAt": expiresAfter(10 * time.Minute), "providerVersion": provider["updatedAt"], "referral": b.Referral}
	if b.Purpose == "bind" {
		flow["userId"] = p.Subject(r).ID
	}
	if c.Err(w, a.C.Put("flows", flow)) {
		return
	}
	options := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if p.Text(provider, "kind") == "google" {
		options = append(options, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, p.Record{"authorizationUrl": providerConfig(provider).AuthCodeURL(state, options...)})
}

func redirectResult(w http.ResponseWriter, r *http.Request, provider p.Record, purpose, ticket, message string) {
	query := url.Values{"mode": {purpose}}
	if message != "" {
		query.Set("error", message)
	} else {
		query.Set("ticket", ticket)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, p.Text(provider, "publicUrl")+"/auth/external?"+query.Encode(), http.StatusSeeOther)
}

func (a *app) callback(w http.ResponseWriter, r *http.Request) {
	provider, e := a.Find("providers", r.PathValue("provider"))
	if e != nil || !p.Bool(provider, "enabled") {
		p.Fail(w, 400, "登录服务未启用")
		return
	}
	flow, e := a.Find("flows", r.URL.Query().Get("state"))
	if e != nil || p.Text(flow, "providerId") != p.Text(provider, "id") || p.Text(flow, "expiresAt") < c.Now() || p.Text(flow, "providerVersion") != p.Text(provider, "updatedAt") {
		redirectResult(w, r, provider, "login", "", "授权已到期或配置已变更，请重新发起")
		return
	}
	purpose := p.Text(flow, "purpose")
	if c.Err(w, a.C.Delete("flows", p.Text(flow, "id"))) {
		return
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		redirectResult(w, r, provider, purpose, "", "授权已取消，请重新选择登录方式")
		return
	}
	profile, e := fetchIdentity(r.Context(), providerConfig(provider), p.Text(provider, "kind"), r.URL.Query().Get("code"), p.Text(flow, "verifier"))
	if e != nil {
		redirectResult(w, r, provider, purpose, "", e.Error())
		return
	}
	if purpose == "bind" {
		ticket := p.ID() + p.ID()
		row := p.Record{"id": ticket, "providerId": provider["id"], "subject": profile.Subject, "displayName": profile.DisplayName, "userId": flow["userId"], "proofDigest": flow["proofDigest"], "expiresAt": expiresAfter(3 * time.Minute)}
		if c.Err(w, a.C.Put("pendingBindings", row)) {
			return
		}
		redirectResult(w, r, provider, purpose, ticket, "")
		return
	}
	userID, needsRegistration, e := a.loginIdentity(provider, profile, p.Text(flow, "referral"))
	if e != nil {
		redirectResult(w, r, provider, purpose, "", e.Error())
		return
	}
	if needsRegistration {
		ticket := p.ID() + p.ID()
		row := p.Record{"id": ticket, "providerId": provider["id"], "providerVersion": provider["updatedAt"], "subject": profile.Subject, "displayName": profile.DisplayName, "proofDigest": flow["proofDigest"], "redirectPath": flow["redirectPath"], "expiresAt": expiresAfter(10 * time.Minute), "referral": flow["referral"]}
		if c.Err(w, a.C.Put("pendingRegistrations", row)) {
			return
		}
		redirectResult(w, r, provider, "register", ticket, "")
		return
	}
	grant, e := a.C.CreateLoginGrant(userID, p.Text(flow, "proofDigest"), p.Text(flow, "redirectPath"))
	if e != nil {
		redirectResult(w, r, provider, purpose, "", "账号暂时无法登录，请联系管理员")
		return
	}
	redirectResult(w, r, provider, purpose, grant.Ticket, "")
}

func (a *app) completeBinding(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	var b struct{ Ticket, Proof string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row, e := a.Find("pendingBindings", b.Ticket)
	if e != nil || p.Text(row, "expiresAt") < c.Now() || p.Text(row, "userId") != p.Subject(r).ID || subtle.ConstantTimeCompare([]byte(p.Text(row, "proofDigest")), []byte(digest(b.Proof))) != 1 {
		p.Fail(w, 400, "绑定请求无效或已到期，请重新发起")
		return
	}
	provider, e := a.Find("providers", p.Text(row, "providerId"))
	if e != nil || !p.Bool(provider, "enabled") {
		p.Fail(w, 400, "此登录服务已停用")
		return
	}
	if c.Err(w, a.storeBinding(p.Text(row, "providerId"), p.Text(row, "subject"), p.Subject(r).ID, p.Text(row, "displayName"), c.Now())) || c.Err(w, a.C.Delete("pendingBindings", b.Ticket)) {
		return
	}
	p.JSON(w, p.Record{"message": "账号已绑定", "redirectPath": "/account"})
}

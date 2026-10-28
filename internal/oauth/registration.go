package oauth

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strings"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) bindingUser(providerID, subject string) (string, error) {
	rows, e := a.C.List("bindings")
	if e != nil {
		return "", e
	}
	for _, row := range rows {
		if p.Text(row, "providerId") == providerID && p.Text(row, "subject") == subject {
			return p.Text(row, "userId"), nil
		}
	}
	return "", nil
}

func (a *app) registrationEnabled() bool {
	var policy struct {
		Enabled bool `json:"enabled"`
	}
	return a.C.Call("registration", "GET", "/internal/policy", nil, &policy) == nil && policy.Enabled
}

func (a *app) registrationAllowed(provider p.Record) bool {
	return p.Bool(provider, "enabled") && p.Bool(provider, "allowRegistration") && a.registrationEnabled()
}

func (a *app) loginIdentity(provider p.Record, profile identity, referral string) (userID string, needsRegistration bool, err error) {
	userID, err = a.bindingUser(p.Text(provider, "id"), profile.Subject)
	if err != nil || userID != "" {
		return userID, false, err
	}
	if !a.registrationAllowed(provider) {
		return "", false, fmt.Errorf("此第三方账号尚未绑定，请先登录已有账号并在账户页绑定")
	}
	if !profile.EmailVerified || !strings.Contains(profile.Email, "@") {
		return "", true, nil
	}
	userID, err = a.registerIdentity(provider, profile, profile.Email, p.ID()+p.ID(), referral)
	return userID, false, err
}

func (a *app) registerIdentity(provider p.Record, profile identity, email, password, referral string) (string, error) {
	if user, e := a.bindingUser(p.Text(provider, "id"), profile.Subject); e != nil || user != "" {
		return user, e
	}
	if !a.registrationAllowed(provider) {
		return "", fmt.Errorf("新账号注册已关闭，请登录已有账号后绑定")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.Contains(email, "@") || len(password) < 10 {
		return "", fmt.Errorf("请输入有效邮箱和至少 10 位密码")
	}
	users, e := a.C.UsersByEmail(email)
	if e != nil {
		return "", e
	}
	for _, user := range users {
		if strings.EqualFold(p.Text(user, "email"), email) {
			return "", fmt.Errorf("该邮箱已有账号，请用原方式登录后绑定")
		}
	}
	row := p.Record{
		"id": bindingID(p.Text(provider, "id"), profile.Subject), "providerId": provider["id"],
		"subject": profile.Subject, "displayName": profile.DisplayName, "createdAt": c.Now(),
		"referral": referral, "registeredAt": c.Now(), "registrationPending": true,
	}
	userID, e := a.C.CreateUserWithRecord(email, profile.DisplayName, password, "bindings", row)
	if e != nil {
		return "", fmt.Errorf("无法创建账号，请稍后重试")
	}
	row["userId"] = userID
	if e = a.PublishRegistration("bindings", row); e != nil {
		log.Printf("oauth registration event pending: %v", e)
	}
	return userID, nil
}

func (a *app) pendingRegistration(ticket, proof string) (row, provider p.Record, err error) {
	row, err = a.Find("pendingRegistrations", ticket)
	if err != nil || len(proof) < 32 || p.Text(row, "expiresAt") < c.Now() || subtle.ConstantTimeCompare([]byte(p.Text(row, "proofDigest")), []byte(digest(proof))) != 1 {
		return nil, nil, fmt.Errorf("注册请求无效或已到期，请重新登录")
	}
	provider, err = a.Find("providers", p.Text(row, "providerId"))
	if err != nil || !p.Bool(provider, "enabled") {
		return nil, nil, fmt.Errorf("此登录服务的注册已关闭或配置已变更")
	}
	userID, err := a.bindingUser(p.Text(provider, "id"), p.Text(row, "subject"))
	if err != nil {
		return nil, nil, err
	}
	if userID == "" && (p.Text(provider, "updatedAt") != p.Text(row, "providerVersion") || !a.registrationAllowed(provider)) {
		return nil, nil, fmt.Errorf("新账号注册已关闭，请登录已有账号后绑定")
	}
	return row, provider, nil
}

func (a *app) registration(w http.ResponseWriter, r *http.Request) {
	var b struct{ Ticket, Proof string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row, provider, e := a.pendingRegistration(b.Ticket, b.Proof)
	if c.Err(w, e) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, p.Record{"providerName": provider["name"], "displayName": row["displayName"], "expiresAt": row["expiresAt"]})
}

func (a *app) completeRegistration(w http.ResponseWriter, r *http.Request) {
	var b struct{ Ticket, Proof, Email, Password string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	row, provider, e := a.pendingRegistration(b.Ticket, b.Proof)
	if c.Err(w, e) {
		return
	}
	userID, e := a.bindingUser(p.Text(provider, "id"), p.Text(row, "subject"))
	if c.Err(w, e) {
		return
	}
	if userID == "" {
		userID, e = a.registerIdentity(provider, identity{Subject: p.Text(row, "subject"), DisplayName: p.Text(row, "displayName")}, b.Email, b.Password, p.Text(row, "referral"))
		if c.Err(w, e) {
			return
		}
	}
	grant, e := a.C.CreateLoginGrant(userID, p.Text(row, "proofDigest"), p.Text(row, "redirectPath"))
	if c.Err(w, e) || c.Err(w, a.C.Delete("pendingRegistrations", b.Ticket)) {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, grant)
}

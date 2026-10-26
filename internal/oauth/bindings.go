package oauth

import (
	"fmt"
	"net/http"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func bindingID(provider, subject string) string { return digest(provider + "\x00" + subject) }

func (a *app) storeBinding(provider, subject, user, name, created string) error {
	rows, e := a.C.List("bindings")
	if e != nil {
		return e
	}
	for _, row := range rows {
		if p.Text(row, "providerId") != provider {
			continue
		}
		if p.Text(row, "subject") == subject && p.Text(row, "userId") == user {
			return nil
		}
		if p.Text(row, "subject") == subject {
			return fmt.Errorf("此第三方账号已绑定其他用户")
		}
		if p.Text(row, "userId") == user {
			return fmt.Errorf("你已绑定此登录服务，请先解除原绑定")
		}
	}
	return a.C.Put("bindings", p.Record{"id": bindingID(provider, subject), "providerId": provider, "subject": subject, "userId": user, "displayName": name, "createdAt": created})
}

func (a *app) bindings(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	providers, e := a.C.List("providers")
	if c.Err(w, e) {
		return
	}
	names := map[string]string{}
	enabled := map[string]bool{}
	for _, provider := range providers {
		names[p.Text(provider, "id")] = p.Text(provider, "name")
		enabled[p.Text(provider, "id")] = p.Bool(provider, "enabled")
	}
	rows, e := a.C.List("bindings")
	if c.Err(w, e) {
		return
	}
	out := []p.Record{}
	available := 0
	for _, row := range rows {
		if p.Text(row, "userId") == p.Subject(r).ID {
			out = append(out, p.Record{"id": row["id"], "providerId": row["providerId"], "providerName": names[p.Text(row, "providerId")], "displayName": row["displayName"], "createdAt": row["createdAt"]})
			if enabled[p.Text(row, "providerId")] {
				available++
			}
		}
	}
	for _, row := range out {
		row["requiresPassword"] = len(out) == 1 || available <= 1 && enabled[p.Text(row, "providerId")]
	}
	w.Header().Set("Cache-Control", "no-store")
	p.JSON(w, out)
}

func (a *app) deleteBinding(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	row, e := a.Find("bindings", r.PathValue("id"))
	if c.Err(w, e) {
		return
	}
	if p.Text(row, "userId") != p.Subject(r).ID {
		p.Fail(w, 403, "无权解除此账号绑定")
		return
	}
	rows, e := a.C.List("bindings")
	if c.Err(w, e) {
		return
	}
	count := 0
	available := 0
	providers, e := a.C.List("providers")
	if c.Err(w, e) {
		return
	}
	enabled := map[string]bool{}
	for _, provider := range providers {
		enabled[p.Text(provider, "id")] = p.Bool(provider, "enabled")
	}
	for _, binding := range rows {
		if p.Text(binding, "userId") == p.Subject(r).ID {
			count++
			if enabled[p.Text(binding, "providerId")] {
				available++
			}
		}
	}
	if count == 1 || available <= 1 && enabled[p.Text(row, "providerId")] {
		var b struct{ Password, Code string }
		if c.Err(w, p.Decode(r, &b)) {
			return
		}
		if c.Err(w, a.C.VerifyUserCredential(p.Subject(r).ID, b.Password, b.Code)) {
			return
		}
	}
	if c.Err(w, a.preserveRegistration(row)) {
		return
	}
	if !c.Err(w, a.C.Delete("bindings", r.PathValue("id"))) {
		p.JSON(w, true)
	}
}

func (a *app) importBindings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b struct {
		DryRun   *bool      `json:"dryRun"`
		Bindings []p.Record `json:"bindings"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	providers, e := a.C.List("providers")
	if c.Err(w, e) {
		return
	}
	providerIDs := map[string]bool{}
	for _, provider := range providers {
		providerIDs[p.Text(provider, "id")] = true
	}
	users, e := a.C.Users()
	if c.Err(w, e) {
		return
	}
	userIDs := map[string]bool{}
	for _, user := range users {
		userIDs[p.Text(user, "id")] = true
	}
	existing, e := a.C.List("bindings")
	if c.Err(w, e) {
		return
	}
	identities, userProviders := map[string]string{}, map[string]string{}
	for _, row := range existing {
		identities[bindingID(p.Text(row, "providerId"), p.Text(row, "subject"))] = p.Text(row, "userId")
		userProviders[p.Text(row, "userId")+"\x00"+p.Text(row, "providerId")] = p.Text(row, "subject")
	}
	prepared, skipped := []p.Record{}, 0
	for i, source := range b.Bindings {
		provider, subject, user := p.Text(source, "providerId"), p.Text(source, "subject"), p.Text(source, "userId")
		if !providerIDs[provider] || !userIDs[user] || subject == "" {
			p.Fail(w, 400, fmt.Sprintf("第 %d 条绑定的服务、目标用户或第三方账号标识无效", i+1))
			return
		}
		id, pair := bindingID(provider, subject), user+"\x00"+provider
		if previous, ok := identities[id]; ok {
			if previous == user {
				skipped++
				continue
			}
			p.Fail(w, 409, fmt.Sprintf("第 %d 条第三方账号已绑定其他目标用户", i+1))
			return
		}
		if previous, ok := userProviders[pair]; ok && previous != subject {
			p.Fail(w, 409, fmt.Sprintf("第 %d 条目标用户已有同服务的其他绑定", i+1))
			return
		}
		created := p.Text(source, "createdAt")
		if created == "" {
			created = c.Now()
		} else if _, e := time.Parse(time.RFC3339, created); e != nil {
			p.Fail(w, 400, fmt.Sprintf("第 %d 条创建时间需为带时区的标准日期", i+1))
			return
		}
		prepared = append(prepared, p.Record{"id": id, "providerId": provider, "subject": subject, "userId": user, "displayName": p.Text(source, "displayName"), "createdAt": created})
		identities[id], userProviders[pair] = user, subject
	}
	dryRun := b.DryRun == nil || *b.DryRun
	if !dryRun && c.Err(w, a.C.PutMany("bindings", prepared)) {
		return
	}
	p.JSON(w, p.Record{"dryRun": dryRun, "newBindings": len(prepared), "existingBindings": skipped, "total": len(b.Bindings)})
}

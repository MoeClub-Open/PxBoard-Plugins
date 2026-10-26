package affiliate

import (
	"net/http"
	"net/url"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) summary(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	user := p.Subject(r).ID
	available, e := a.available(user)
	if c.Err(w, e) {
		return
	}
	commissions, e := a.query("commissions", equal("userId", user))
	if c.Err(w, e) {
		return
	}
	total, pending := 0.0, 0.0
	for _, row := range commissions {
		if c.Allowed(p.Text(row, "status"), "reversed", "historical") {
			continue
		}
		total += p.Number(row, "amount")
		if p.Text(row, "status") == "pending" {
			pending += p.Number(row, "amount")
		}
	}
	referrals, e := a.query("referrals", equal("parentId", user))
	if c.Err(w, e) {
		return
	}
	config, e := a.config()
	if c.Err(w, e) {
		return
	}
	profile, e := a.profile(user)
	if c.Err(w, e) {
		return
	}
	percent, firstOnly := terms(config, profile)
	mode := "recurring"
	if firstOnly {
		mode = "firstPurchase"
	}
	p.JSON(w, p.Record{"availableAmount": available, "pendingAmount": pending, "totalAmount": total, "invitedCount": len(referrals), "commissionPercent": percent, "commissionHoldDays": config["commissionHoldDays"], "commissionMode": mode, "referralPath": "/register?ref=" + url.QueryEscape(user)})
}

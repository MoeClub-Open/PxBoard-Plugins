package redeem

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func claimID(code, user, request string) string {
	digest := sha256.Sum256([]byte(code + "\x00" + user + "\x00" + request))
	return hex.EncodeToString(digest[:])
}

func (a *app) redeem(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) || c.Err(w, a.ready()) {
		return
	}
	var body struct{ Code, RequestID string }
	if c.Err(w, p.Decode(r, &body)) {
		return
	}
	code, err := a.find("codes", "code", strings.TrimSpace(body.Code))
	if c.Err(w, err) {
		return
	}
	if code == nil {
		p.Fail(w, 400, "兑换码不存在")
		return
	}
	user, id := p.Subject(r).ID, p.Text(code, "id")
	personal, err := a.query("claims", 0, 1000, equal("codeId", id), equal("userId", user))
	if c.Err(w, err) {
		return
	}
	for _, claim := range personal.Items {
		if p.Text(claim, "status") == "pending" || p.Number(code, "perUserLimit") == 1 || body.RequestID != "" && p.Text(claim, "id") == claimID(id, user, body.RequestID) {
			a.respondClaim(w, claim)
			return
		}
	}
	if body.RequestID == "" {
		if p.Number(code, "perUserLimit") != 1 {
			p.Fail(w, 400, "请重新发起兑换请求")
			return
		}
		body.RequestID = "once"
	}
	all, err := a.query("claims", 0, 1, equal("codeId", id))
	if c.Err(w, err) {
		return
	}
	expires, _ := time.Parse(time.RFC3339, p.Text(code, "expiresAt"))
	if !p.Bool(code, "enabled") || !expires.IsZero() && !expires.After(time.Now()) || p.Number(code, "limit") > 0 && p.Number(code, "historicalUses")+float64(all.Total) >= p.Number(code, "limit") || p.Number(code, "perUserLimit") > 0 && float64(personal.Total) >= p.Number(code, "perUserLimit") {
		p.Fail(w, 400, "兑换码已停用、到期或达到兑换次数")
		return
	}
	if p.Text(code, "kind") == "balance" && !a.walletAvailable() {
		p.Fail(w, 400, "本站尚未启用余额兑换")
		return
	}
	if c.Err(w, a.checkEligibility(code, user)) {
		return
	}
	claim := p.Record{"id": claimID(id, user, body.RequestID), "codeId": id, "userId": user, "name": code["name"], "kind": code["kind"], "summary": codeSummary(code), "status": "pending", "createdAt": c.Now()}
	claim["redemptionGroup"] = code["redemptionGroup"]
	claim["reward"] = rewardFor(code, claim)
	if c.Err(w, a.C.Put("claims", claim)) {
		return
	}
	a.respondClaim(w, claim)
}

func (a *app) respondClaim(w http.ResponseWriter, claim p.Record) {
	if p.Text(claim, "status") == "redeemed" {
		p.JSON(w, p.Record{"id": claim["id"], "status": "redeemed", "message": "兑换已完成"})
		return
	}
	if err := a.completeClaim(claim); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		p.JSON(w, p.Record{"id": claim["id"], "status": "pending", "message": "兑换已登记，正在处理，可稍后重试"})
		return
	}
	p.JSON(w, p.Record{"id": claim["id"], "status": "redeemed", "message": "兑换成功"})
}

func (a *app) completeClaim(claim p.Record) error {
	claim["attemptedAt"] = c.Now()
	if err := a.C.Put("claims", claim); err != nil {
		return err
	}
	reward := object(claim["reward"])
	err := a.deliverReward(claim, reward)
	if err != nil {
		claim["lastError"] = err.Error()
		_ = a.C.Put("claims", claim)
		return err
	}
	claim["status"], claim["redeemedAt"] = "redeemed", c.Now()
	delete(claim, "lastError")
	return a.C.Put("claims", claim)
}

func (a *app) event(event p.Event) error {
	if event.Source != "core" || event.Type != "system.tick" {
		return nil
	}
	if err := a.ready(); err != nil {
		return nil
	}
	claims, err := a.all("claims", equal("status", "pending"))
	if err != nil {
		return err
	}
	sort.Slice(claims, func(i, j int) bool { return p.Text(claims[i], "attemptedAt") < p.Text(claims[j], "attemptedAt") })
	if len(claims) > 0 {
		return a.completeClaim(claims[0])
	}
	return nil
}

func (a *app) claims(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	filters := []p.Record{}
	if !p.Admin(r) || r.URL.Query().Get("mine") == "1" {
		filters = append(filters, equal("userId", p.Subject(r).ID))
	}
	claims, err := a.all("claims", filters...)
	if c.Err(w, err) {
		return
	}
	result := []p.Record{}
	for _, claim := range claims {
		row := p.Record{}
		for _, field := range []string{"id", "name", "kind", "summary", "status", "createdAt", "redeemedAt"} {
			row[field] = claim[field]
		}
		if p.Admin(r) {
			row["userId"], row["lastError"] = claim["userId"], claim["lastError"]
		} else if p.Text(claim, "lastError") != "" {
			row["lastError"] = "兑换处理中，请稍后重试"
		}
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool { return p.Text(result[i], "createdAt") > p.Text(result[j], "createdAt") })
	p.JSON(w, result)
}

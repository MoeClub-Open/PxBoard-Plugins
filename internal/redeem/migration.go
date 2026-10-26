package redeem

import (
	"fmt"
	"math"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

const migrationKey = "legacy-migration"

func (a *app) migrationAPI() {
	a.Mux.HandleFunc("GET /migration", a.migrationStatus)
	a.Mux.HandleFunc("POST /migration/start", a.startMigration)
	a.Mux.HandleFunc("POST /migration/import", a.importMigration)
	a.Mux.HandleFunc("POST /migration/finish", a.finishMigration)
}

func (a *app) ready() error {
	state, err := a.find("settings", "id", migrationKey)
	if err != nil {
		return err
	}
	if p.Text(state, "status") == "importing" {
		return fmt.Errorf("兑换码数据升级中，请稍后重试")
	}
	return nil
}

func (a *app) migrationStatus(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	state, err := a.find("settings", "id", migrationKey)
	if !c.Err(w, err) {
		p.JSON(w, state)
	}
}

func (a *app) startMigration(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	if p.Text(b, "migrationId") == "" || p.Number(b, "codeCount") < 0 || p.Number(b, "claimCount") < 0 || math.Trunc(p.Number(b, "codeCount")) != p.Number(b, "codeCount") || math.Trunc(p.Number(b, "claimCount")) != p.Number(b, "claimCount") {
		p.Fail(w, 400, "升级任务标识或预计数量无效")
		return
	}
	old, err := a.find("settings", "id", migrationKey)
	if c.Err(w, err) {
		return
	}
	if old != nil {
		if old["migrationId"] == b["migrationId"] && p.Number(old, "codeCount") == p.Number(b, "codeCount") && p.Number(old, "claimCount") == p.Number(b, "claimCount") {
			p.JSON(w, old)
			return
		}
		p.Fail(w, 409, "已有其他兑换码升级任务，请核对原任务后继续")
		return
	}
	state := p.Record{"id": migrationKey, "migrationId": b["migrationId"], "codeCount": b["codeCount"], "claimCount": b["claimCount"], "status": "importing", "createdAt": c.Now()}
	if !c.Err(w, a.C.Put("settings", state)) {
		p.JSON(w, state)
	}
}

func (a *app) importMigration(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b struct {
		MigrationID string     `json:"migrationId"`
		DryRun      *bool      `json:"dryRun"`
		CodeIDs     []string   `json:"codeIds"`
		Codes       []p.Record `json:"codes"`
		Claims      []p.Record `json:"claims"`
	}
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	dryRun := b.DryRun == nil || *b.DryRun
	state, err := a.find("settings", "id", migrationKey)
	if c.Err(w, err) {
		return
	}
	if b.MigrationID == "" || !dryRun && (state == nil || p.Text(state, "migrationId") != b.MigrationID) {
		p.Fail(w, 409, "请先开始同一标识的升级任务")
		return
	}
	codes, err := a.all("codes")
	if c.Err(w, err) {
		return
	}
	byID, byValue := map[string]p.Record{}, map[string]string{}
	for _, code := range codes {
		byID[p.Text(code, "id")], byValue[p.Text(code, "code")] = code, p.Text(code, "id")
	}
	known := map[string]bool{}
	if dryRun {
		for _, id := range b.CodeIDs {
			known[id] = true
		}
	}
	newCodes, newClaims := []p.Record{}, []p.Record{}
	for _, code := range b.Codes {
		id := p.Text(code, "id")
		if id == "" || validateCode(code) != nil {
			p.Fail(w, 400, "升级记录包含无效兑换码："+id)
			return
		}
		if previous := byValue[p.Text(code, "code")]; previous != "" && previous != id {
			p.Fail(w, 409, "相同兑换码对应不同记录："+id)
			return
		}
		if old := byID[id]; old != nil {
			if old["code"] != code["code"] || !dryRun && p.Text(old, "migrationId") != b.MigrationID {
				p.Fail(w, 409, "兑换码标识已被其他记录使用："+id)
				return
			}
		} else {
			code["migrationId"] = b.MigrationID
			newCodes = append(newCodes, code)
		}
		byID[id], byValue[p.Text(code, "code")], known[id] = code, id, true
	}
	users := map[string]bool{}
	if len(b.Claims) > 0 {
		rows, err := a.C.Users()
		if c.Err(w, err) {
			return
		}
		for _, user := range rows {
			users[p.Text(user, "id")] = true
		}
	}
	seen := map[string]p.Record{}
	for _, claim := range b.Claims {
		id, codeID, user := p.Text(claim, "id"), p.Text(claim, "codeId"), p.Text(claim, "userId")
		if id == "" || !users[user] || byID[codeID] == nil && !known[codeID] {
			p.Fail(w, 400, "升级兑换记录缺少码或目标用户："+id)
			return
		}
		if err = validateReward(claim); c.Err(w, err) {
			return
		}
		old := seen[id]
		if old == nil {
			old, err = a.find("claims", "id", id)
			if c.Err(w, err) {
				return
			}
		}
		if old != nil {
			if old["codeId"] != codeID || old["userId"] != user || !dryRun && p.Text(old, "migrationId") != b.MigrationID {
				p.Fail(w, 409, "兑换记录标识已被其他记录使用："+id)
				return
			}
		} else {
			claim["migrationId"] = b.MigrationID
			newClaims = append(newClaims, claim)
		}
		seen[id] = claim
	}
	if !dryRun && p.Text(state, "status") == "completed" && (len(newCodes) > 0 || len(newClaims) > 0) {
		p.Fail(w, 409, "该升级任务已完成，不能追加不同记录")
		return
	}
	if !dryRun && (c.Err(w, a.C.PutMany("codes", newCodes)) || c.Err(w, a.C.PutMany("claims", newClaims))) {
		return
	}
	p.JSON(w, p.Record{"dryRun": dryRun, "newCodes": len(newCodes), "newClaims": len(newClaims)})
}

func (a *app) finishMigration(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var b struct{ MigrationID string }
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	state, err := a.find("settings", "id", migrationKey)
	if c.Err(w, err) {
		return
	}
	if state == nil || p.Text(state, "migrationId") != b.MigrationID {
		p.Fail(w, 409, "升级任务不存在或标识不同")
		return
	}
	codes, err := a.query("codes", 0, 1, equal("migrationId", b.MigrationID))
	if c.Err(w, err) {
		return
	}
	claims, err := a.query("claims", 0, 1, equal("migrationId", b.MigrationID))
	if c.Err(w, err) {
		return
	}
	if codes.Total != int(p.Number(state, "codeCount")) || claims.Total != int(p.Number(state, "claimCount")) {
		p.Fail(w, 409, "升级数据尚不完整，请使用同一备份继续搬入")
		return
	}
	state["status"], state["completedAt"] = "completed", c.Now()
	if !c.Err(w, a.C.Put("settings", state)) {
		p.JSON(w, state)
	}
}

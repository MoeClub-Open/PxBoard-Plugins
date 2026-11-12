package admob

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

type app struct {
	*c.App
	keys *verifierKeyCache
}

func New() http.Handler {
	a := &app{App: c.New(), keys: newVerifierKeyCache()}
	a.Mux.HandleFunc("GET /settings", a.settings)
	a.Mux.HandleFunc("PATCH /settings", a.settings)
	a.Mux.HandleFunc("GET /logs", a.logs)
	a.Mux.HandleFunc("GET /ssv", a.ssv)
	a.RPC("reward.prepare", a.rewardPrepare)
	a.RPC("reward.pending", a.rewardPending)
	a.RPC("reward.history", a.rewardHistory)
	a.RPC("rewards.list", a.rewardHistory)
	a.View("admob", a.view)
	return a
}

func defaultSettings() p.Record {
	return p.Record{
		"id":                    "settings",
		"ssvSecret":             "",
		"planEnabled":           false,
		"planAdUnitId":          "",
		"planDurationDays":      1.0,
		"planQuotaGiB":          5.0,
		"planGroups":            "default",
		"planDeviceLimit":       0.0,
		"planSpeedLimitMbps":    0.0,
		"planRequiresInactive":  true,
		"pointsEnabled":         false,
		"pointsAdUnitId":        "",
		"pointsAmount":          200.0,
		"tokenTtlSeconds":       900.0,
		"callbackMaxAgeSeconds": 3600.0,
	}
}

func (a *app) ensureConfig() (p.Record, error) {
	rows, err := a.C.List("settings")
	if err != nil {
		return nil, err
	}
	var cfg p.Record
	for _, row := range rows {
		if p.Text(row, "id") == "settings" {
			cfg = row
			break
		}
	}
	changed := cfg == nil
	if cfg == nil {
		cfg = defaultSettings()
	} else {
		c.Defaults(cfg, nil, defaultSettings())
	}
	if strings.TrimSpace(p.Text(cfg, "ssvSecret")) == "" {
		sec, err := randomSecretHex(32)
		if err != nil {
			return nil, err
		}
		cfg["ssvSecret"] = sec
		changed = true
	}
	if changed {
		if err := a.C.Put("settings", cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (a *app) settings(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	cfg, err := a.ensureConfig()
	if c.Err(w, err) {
		return
	}
	if r.Method == http.MethodGet {
		p.JSON(w, []p.Record{cfg})
		return
	}
	var b p.Record
	if c.Err(w, p.Decode(r, &b)) {
		return
	}
	delete(b, "id")
	for k, v := range b {
		cfg[k] = v
	}
	if strings.TrimSpace(p.Text(cfg, "ssvSecret")) == "" {
		sec, genErr := randomSecretHex(32)
		if c.Err(w, genErr) {
			return
		}
		cfg["ssvSecret"] = sec
	}
	if p.Number(cfg, "planDurationDays") < 1 || math.Trunc(p.Number(cfg, "planDurationDays")) != p.Number(cfg, "planDurationDays") {
		p.Fail(w, 400, "赠送有效天数须为正整数")
		return
	}
	if p.Number(cfg, "planQuotaGiB") <= 0 {
		p.Fail(w, 400, "赠送套餐流量须大于 0")
		return
	}
	if p.Bool(cfg, "pointsEnabled") && (p.Number(cfg, "pointsAmount") <= 0 || math.Trunc(p.Number(cfg, "pointsAmount")) != p.Number(cfg, "pointsAmount")) {
		p.Fail(w, 400, "每次奖励余额须为正整数分")
		return
	}
	if strings.TrimSpace(p.Text(cfg, "planGroups")) == "" {
		cfg["planGroups"] = "default"
	}
	if !c.Err(w, a.C.Put("settings", cfg)) {
		p.JSON(w, cfg)
	}
}

func (a *app) walletAvailable() bool {
	var result struct {
		WalletCredits bool `json:"walletCredits"`
	}
	return a.C.Call("commerce", "GET", "/internal/capabilities", nil, &result) == nil && result.WalletCredits
}

func (a *app) userAccount(userID string) (p.Record, error) {
	var account p.Record
	if err := a.C.Call("access", "GET", "/internal/accounts/"+url.PathEscape(userID), nil, &account); err != nil {
		return nil, err
	}
	return account, nil
}

func (a *app) accountStillActive(userID string) (bool, error) {
	account, err := a.userAccount(userID)
	if err != nil {
		return false, err
	}
	return len(account) > 0 && p.Bool(account, "effective"), nil
}

func shortHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:16])
}

func (a *app) rewardPrepare(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.Member(w, r) {
		return
	}
	var req struct {
		Scene string `json:"scene"`
	}
	if err := p.Decode(r, &req); err != nil && !errors.Is(err, io.EOF) {
		p.Fail(w, 400, "请求格式无效")
		return
	}
	cfg, err := a.ensureConfig()
	if c.Err(w, err) {
		return
	}
	subject := p.Subject(r)
	ttl := int(p.Number(cfg, "tokenTtlSeconds"))
	secret := p.Text(cfg, "ssvSecret")

	planEnabled := p.Bool(cfg, "planEnabled") && int(p.Number(cfg, "planDurationDays")) >= 1 && p.Number(cfg, "planQuotaGiB") > 0
	if planEnabled && p.Bool(cfg, "planRequiresInactive") {
		active, activeErr := a.accountStillActive(subject.ID)
		if activeErr != nil {
			if req.Scene == ScenePlan {
				c.Err(w, activeErr)
				return
			}
		} else if active {
			if req.Scene == ScenePlan {
				p.Fail(w, 400, "当前套餐仍可用，暂无需领取广告套餐")
				return
			}
			planEnabled = false
		}
	}
	pointsEnabled := p.Bool(cfg, "pointsEnabled") && p.Number(cfg, "pointsAmount") > 0 && a.walletAvailable()

	out := p.Record{
		"planRewardAdEnabled":   planEnabled,
		"planAdUnitId":          strings.TrimSpace(p.Text(cfg, "planAdUnitId")),
		"planSsvUserId":         "",
		"planSsvCustomData":     "",
		"pointsRewardAdEnabled": pointsEnabled,
		"pointsAdUnitId":        strings.TrimSpace(p.Text(cfg, "pointsAdUnitId")),
		"pointsSsvUserId":       "",
		"pointsSsvCustomData":   "",
	}
	if planEnabled && (req.Scene == "" || req.Scene == ScenePlan) {
		tok, err := makeCustomData(secret, subject.ID, ScenePlan, ttl)
		if c.Err(w, err) {
			return
		}
		out["planSsvUserId"] = subject.ID
		out["planSsvCustomData"] = tok
	}
	if pointsEnabled && (req.Scene == "" || req.Scene == ScenePoints) {
		tok, err := makeCustomData(secret, subject.ID, ScenePoints, ttl)
		if c.Err(w, err) {
			return
		}
		out["pointsSsvUserId"] = subject.ID
		out["pointsSsvCustomData"] = tok
	}
	p.JSON(w, out)
}

func formatLogRecord(row p.Record) p.Record {
	createdAt := p.Text(row, "createdAt")
	var createdUnix, usedUnix int64
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		createdUnix = t.Unix()
	}
	if p.Text(row, "status") == "credited" {
		if t, err := time.Parse(time.RFC3339, p.Text(row, "creditedAt")); err == nil {
			usedUnix = t.Unix()
		} else {
			usedUnix = createdUnix
		}
	}
	return p.Record{
		"id":            row["id"],
		"scene":         row["scene"],
		"transactionId": row["transactionId"],
		"status":        row["status"],
		"error":         p.Text(row, "error"),
		"rewardContent": p.Text(row, "rewardContent"),
		"createdAt":     createdAt,
		"createdAtUnix": createdUnix,
		"usedAtUnix":    usedUnix,
		"credited":      p.Text(row, "status") == "credited",
	}
}

func (a *app) rewardPending(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.Member(w, r) {
		return
	}
	var req struct {
		CustomData      string `json:"customData"`
		CustomDataSnake string `json:"custom_data"`
	}
	if c.Err(w, p.Decode(r, &req)) {
		return
	}
	customData := strings.TrimSpace(req.CustomData)
	if customData == "" {
		customData = strings.TrimSpace(req.CustomDataSnake)
	}
	cfg, err := a.ensureConfig()
	if c.Err(w, err) {
		return
	}
	claims, err := verifyCustomData(p.Text(cfg, "ssvSecret"), customData, true, 0)
	if c.Err(w, err) {
		return
	}
	subject := p.Subject(r)
	if claims.UserID != subject.ID {
		p.Fail(w, 403, "AdMob SSV 令牌所属用户不匹配")
		return
	}
	existing, err := a.QueryAll("logs", p.Record{"field": "customData", "value": customData})
	if c.Err(w, err) {
		return
	}
	for _, row := range existing {
		if p.Text(row, "status") == "credited" {
			p.JSON(w, formatLogRecord(row))
			return
		}
	}
	if len(existing) > 0 {
		p.JSON(w, formatLogRecord(existing[0]))
		return
	}
	if claims.Scene == ScenePlan && p.Bool(cfg, "planRequiresInactive") {
		active, err := a.accountStillActive(subject.ID)
		if c.Err(w, err) {
			return
		}
		if active {
			p.Fail(w, 400, "当前套餐仍可用，暂无需领取广告套餐")
			return
		}
	}
	row := p.Record{
		"id":            "pending-" + shortHash(customData),
		"userId":        subject.ID,
		"scene":         claims.Scene,
		"transactionId": "pending:" + claims.Scene + ":" + shortHash(customData),
		"customData":    customData,
		"status":        "pending",
		"error":         "",
		"rewardContent": "",
		"createdAt":     c.Now(),
	}
	if c.Err(w, a.C.Put("logs", row)) {
		return
	}
	p.JSON(w, formatLogRecord(row))
}

func (a *app) rewardHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !c.Member(w, r) {
		return
	}
	rows, err := a.QueryAll("logs", p.Record{"field": "userId", "value": p.Subject(r).ID})
	if c.Err(w, err) {
		return
	}
	slices.SortFunc(rows, func(x, y p.Record) int {
		return cmp.Compare(p.Text(y, "createdAt"), p.Text(x, "createdAt"))
	})
	if len(rows) > 10 {
		rows = rows[:10]
	}
	out := make([]p.Record, 0, len(rows))
	for _, row := range rows {
		out = append(out, formatLogRecord(row))
	}
	p.JSON(w, out)
}

func (a *app) logs(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	result, err := a.ReadRecords("logs", r, nil, "userId", "transactionId", "scene", "status")
	if c.Err(w, err) {
		return
	}
	slices.SortFunc(result.Items, func(x, y p.Record) int {
		return cmp.Compare(p.Text(y, "createdAt"), p.Text(x, "createdAt"))
	})
	c.ListResponse(w, r, result)
}

func (a *app) ssv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	cfg, err := a.ensureConfig()
	if c.Err(w, err) {
		return
	}
	sigValue, keyID, err := a.keys.verifySignature(r.URL.RawQuery)
	if err != nil {
		a.markFailed(r.URL.Query().Get("custom_data"), err.Error())
		p.Fail(w, 400, err.Error())
		return
	}
	q := r.URL.Query()
	cbUserID := strings.TrimSpace(q.Get("user_id"))
	cbCustomData := strings.TrimSpace(q.Get("custom_data"))
	if cbCustomData == "" || cbUserID == ConsoleVerifyToken || cbCustomData == ConsoleVerifyToken {
		p.JSON(w, p.Record{"credited": false, "consoleVerification": true})
		return
	}
	for _, key := range []string{"ad_unit", "timestamp", "transaction_id"} {
		if strings.TrimSpace(q.Get(key)) == "" {
			msg := "AdMob SSV 缺少参数：" + key
			a.markFailed(cbCustomData, msg)
			p.Fail(w, 400, msg)
			return
		}
	}
	rawTS, tsErr := strconv.ParseInt(strings.TrimSpace(q.Get("timestamp")), 10, 64)
	if tsErr != nil {
		a.markFailed(cbCustomData, "AdMob SSV 时间戳无效")
		p.Fail(w, 400, "AdMob SSV 时间戳无效")
		return
	}
	cbSec := normalizeCallbackTimestamp(rawTS)
	maxAge := int64(max(60.0, p.Number(cfg, "callbackMaxAgeSeconds")))
	nowSec := time.Now().Unix()
	if nowSec-cbSec > maxAge || cbSec-nowSec > maxAge {
		a.markFailed(cbCustomData, "AdMob SSV 回调时间超出允许范围")
		p.Fail(w, 400, "AdMob SSV 回调时间超出允许范围")
		return
	}
	claims, err := verifyCustomData(p.Text(cfg, "ssvSecret"), cbCustomData, true, cbSec)
	if err != nil {
		a.markFailed(cbCustomData, err.Error())
		p.Fail(w, 400, err.Error())
		return
	}
	if cbUserID != "" && cbUserID != claims.UserID {
		a.markFailed(cbCustomData, "AdMob SSV user_id 与 custom_data 不匹配")
		p.Fail(w, 400, "AdMob SSV user_id 与 custom_data 不匹配")
		return
	}
	expectedUnit := p.Text(cfg, "planAdUnitId")
	sceneEnabled := p.Bool(cfg, "planEnabled")
	if claims.Scene == ScenePoints {
		expectedUnit = p.Text(cfg, "pointsAdUnitId")
		sceneEnabled = p.Bool(cfg, "pointsEnabled") && a.walletAvailable()
	}
	if !sceneEnabled {
		a.markFailed(cbCustomData, "对应激励广告场景未开启")
		p.Fail(w, 400, "对应激励广告场景未开启")
		return
	}
	actualUnit := strings.TrimSpace(q.Get("ad_unit"))
	if !verifyAdUnit(expectedUnit, actualUnit) {
		a.markFailed(cbCustomData, "AdMob SSV 广告单元不匹配")
		p.Fail(w, 400, "AdMob SSV 广告单元不匹配")
		return
	}

	txID := strings.TrimSpace(q.Get("transaction_id"))
	docID := "tx-" + shortHash(txID)
	if existing, err := a.Find("logs", docID); err == nil && p.Text(existing, "status") == "credited" {
		out := formatLogRecord(existing)
		out["duplicate"] = true
		p.JSON(w, out)
		return
	}
	if sameCustom, err := a.QueryAll("logs", p.Record{"field": "customData", "value": cbCustomData}); err == nil {
		for _, item := range sameCustom {
			if p.Text(item, "status") == "credited" {
				out := formatLogRecord(item)
				out["duplicate"] = true
				p.JSON(w, out)
				return
			}
		}
	}

	rewardContent, deliverErr := a.deliver(cfg, claims.UserID, claims.Scene, txID)
	nowISO := c.Now()
	pendingID := "pending-" + shortHash(cbCustomData)
	createdAt := nowISO
	if pendingRow, err := a.Find("logs", pendingID); err == nil && p.Text(pendingRow, "createdAt") != "" {
		createdAt = p.Text(pendingRow, "createdAt")
	}
	if deliverErr != nil {
		failed := p.Record{
			"id":            docID,
			"userId":        claims.UserID,
			"scene":         claims.Scene,
			"transactionId": txID,
			"adUnit":        actualUnit,
			"customData":    cbCustomData,
			"keyId":         keyID,
			"signature":     sigValue,
			"status":        "failed",
			"error":         deliverErr.Error(),
			"rewardContent": "",
			"createdAt":     createdAt,
		}
		_ = a.C.Delete("logs", pendingID)
		_ = a.C.Put("logs", failed)
		p.Fail(w, 400, deliverErr.Error())
		return
	}

	credited := p.Record{
		"id":            docID,
		"userId":        claims.UserID,
		"scene":         claims.Scene,
		"transactionId": txID,
		"adUnit":        actualUnit,
		"customData":    cbCustomData,
		"keyId":         keyID,
		"signature":     sigValue,
		"status":        "credited",
		"error":         "",
		"rewardContent": rewardContent,
		"createdAt":     createdAt,
		"creditedAt":    nowISO,
	}
	_ = a.C.Delete("logs", pendingID)
	if c.Err(w, a.C.Put("logs", credited)) {
		return
	}
	p.JSON(w, formatLogRecord(credited))
}

func (a *app) deliver(cfg p.Record, userID, scene, txID string) (string, error) {
	operationID := "admob-" + txID
	if scene == ScenePoints {
		amount := int64(math.Round(p.Number(cfg, "pointsAmount")))
		if amount <= 0 {
			return "", fmt.Errorf("积分激励奖励金额未配置")
		}
		if err := a.C.Call("commerce", "POST", "/internal/wallet/credits", p.Record{
			"userId":      userID,
			"amount":      amount,
			"operationId": operationID,
			"reason":      "激励广告奖励",
		}, nil); err != nil {
			return "", err
		}
		return fmt.Sprintf("余额 +%.2f", float64(amount)/100.0), nil
	}
	account, err := a.userAccount(userID)
	if err != nil {
		return "", err
	}
	active := len(account) > 0 && p.Bool(account, "effective")
	if p.Bool(cfg, "planRequiresInactive") && active {
		return "", fmt.Errorf("当前套餐仍可用，暂无需领取广告套餐")
	}
	days := int(math.Round(p.Number(cfg, "planDurationDays")))
	quotaGiB := p.Number(cfg, "planQuotaGiB")
	quotaLabel := strconv.FormatFloat(quotaGiB, 'f', -1, 64)
	if active {
		if p.Text(account, "expiresAt") != "" {
			if err := a.C.Call("access", "POST", "/internal/grant", p.Record{
				"userId":        userID,
				"grantId":       operationID + "-time",
				"kind":          "time",
				"durationUnit":  "day",
				"durationValue": days,
			}, nil); err != nil {
				return "", err
			}
		}
		if !p.Bool(account, "unlimited") {
			if err := a.C.Call("access", "POST", "/internal/grant", p.Record{
				"userId":   userID,
				"grantId":  operationID + "-traffic",
				"kind":     "traffic",
				"quotaGiB": quotaGiB,
			}, nil); err != nil {
				return "", err
			}
		}
		return fmt.Sprintf("套餐延长 %d 天 · 流量 +%s GB", days, quotaLabel), nil
	}
	groups := strings.TrimSpace(p.Text(cfg, "planGroups"))
	if groups == "" {
		groups = "default"
	}
	grant := p.Record{
		"userId":        userID,
		"grantId":       operationID,
		"kind":          "access",
		"durationUnit":  "day",
		"durationValue": days,
		"quotaGiB":      quotaGiB,
		"groups":        groups,
		"preserveUsage": false,
	}
	if dev := p.Number(cfg, "planDeviceLimit"); dev > 0 {
		grant["deviceLimit"] = dev
	}
	if spd := p.Number(cfg, "planSpeedLimitMbps"); spd > 0 {
		grant["speedLimitMbps"] = spd
	}
	if err := a.C.Call("access", "POST", "/internal/grant", grant, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("套餐有效期 %d 天 · 流量 %s GB", days, quotaLabel), nil
}

func (a *app) markFailed(customData, reason string) {
	customData = strings.TrimSpace(customData)
	if customData == "" {
		return
	}
	pendingID := "pending-" + shortHash(customData)
	if row, err := a.Find("logs", pendingID); err == nil && p.Text(row, "status") == "pending" {
		row["status"] = "failed"
		row["error"] = reason
		_ = a.C.Put("logs", row)
	}
}

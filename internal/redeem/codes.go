package redeem

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func validateCode(code p.Record) error {
	if err := c.Validate(code, "code", "name", "kind"); err != nil {
		return err
	}
	if !c.Allowed(p.Text(code, "kind"), "access", "traffic", "time", "reset", "balance") {
		return fmt.Errorf("兑换内容无效")
	}
	for _, field := range []string{"limit", "perUserLimit", "historicalUses"} {
		value := p.Number(code, field)
		if value < 0 || math.Trunc(value) != value {
			return fmt.Errorf("兑换次数须为非负整数")
		}
	}
	if expires := p.Text(code, "expiresAt"); expires != "" {
		value, err := time.Parse(time.RFC3339, expires)
		if err != nil {
			return fmt.Errorf("兑换码到期时间无效")
		}
		code["expiresAt"] = value.UTC().Format(time.RFC3339)
	}
	kind := p.Text(code, "kind")
	if kind == "balance" {
		if amount := p.Number(code, "amount"); amount <= 0 || math.Trunc(amount) != amount {
			return fmt.Errorf("余额金额须为正整数分")
		}
	}
	if kind == "access" || kind == "time" {
		unit, duration := p.Text(code, "durationUnit"), p.Number(code, "durationValue")
		if unit != "permanent" && (!c.Allowed(unit, "day", "month") || duration <= 0 || math.Trunc(duration) != duration) {
			return fmt.Errorf("请填写有效的期限")
		}
	}
	if kind == "access" || kind == "traffic" {
		if p.Number(code, "quotaGiB") < 0 || kind == "traffic" && p.Number(code, "quotaGiB") == 0 {
			return fmt.Errorf("流量额度无效")
		}
	}
	if kind == "access" && strings.TrimSpace(p.Text(code, "groups")) == "" {
		return fmt.Errorf("请填写节点分组")
	}
	return nil
}

func (a *app) normalizeCode(_ *http.Request, values, old p.Record) error {
	if err := a.ready(); err != nil {
		return err
	}
	c.Defaults(values, old, p.Record{"kind": "access", "code": p.ID(), "limit": 1, "perUserLimit": 1, "historicalUses": 0, "enabled": true, "durationUnit": "day", "durationValue": 30, "quotaGiB": 20, "groups": "default", "expiresAt": ""})
	if old != nil {
		if p.Text(values, "code") != "" && values["code"] != old["code"] {
			return fmt.Errorf("兑换码创建后不可修改，请另建兑换码")
		}
		values["code"], values["historicalUses"] = old["code"], old["historicalUses"]
	} else {
		values["historicalUses"] = 0
		if strings.TrimSpace(p.Text(values, "code")) == "" {
			values["code"] = p.ID()
		}
	}
	values["code"] = strings.TrimSpace(p.Text(values, "code"))
	if err := validateCode(values); err != nil {
		return err
	}
	if p.Text(values, "kind") == "balance" && !a.walletAvailable() {
		return fmt.Errorf("余额兑换码需要安装并启用商业运营插件")
	}
	match, err := a.find("codes", "code", p.Text(values, "code"))
	if err != nil {
		return err
	}
	if match != nil && (old == nil || match["id"] != old["id"]) {
		return fmt.Errorf("兑换码已存在")
	}
	return nil
}

func (a *app) saveCode(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	var values p.Record
	if c.Err(w, p.Decode(r, &values)) {
		return
	}
	var old p.Record
	if r.Method == "PATCH" {
		var err error
		old, err = a.find("codes", "id", r.PathValue("id"))
		if c.Err(w, err) {
			return
		}
		if old == nil {
			p.Fail(w, 404, "兑换码不存在")
			return
		}
	}
	if c.Err(w, a.normalizeCode(r, values, old)) {
		return
	}
	if old == nil {
		values["id"], values["createdAt"] = p.ID(), c.Now()
	} else {
		for key, value := range values {
			old[key] = value
		}
		values = old
		values["id"] = r.PathValue("id")
	}
	if !c.Err(w, a.C.Put("codes", values)) {
		p.JSON(w, values)
	}
}

func (a *app) codes(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) {
		return
	}
	codes, err := a.all("codes")
	if c.Err(w, err) {
		return
	}
	claims, err := a.all("claims")
	if c.Err(w, err) {
		return
	}
	counts := map[string]int{}
	for _, claim := range claims {
		counts[p.Text(claim, "codeId")]++
	}
	for _, code := range codes {
		code["used"] = p.Number(code, "historicalUses") + float64(counts[p.Text(code, "id")])
		code["summary"] = codeSummary(code)
	}
	sort.Slice(codes, func(i, j int) bool { return p.Text(codes[i], "createdAt") > p.Text(codes[j], "createdAt") })
	p.JSON(w, codes)
}

func (a *app) deleteCode(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r) || c.Err(w, a.ready()) {
		return
	}
	claims, err := a.query("claims", 0, 1, equal("codeId", r.PathValue("id")))
	if c.Err(w, err) {
		return
	}
	if claims.Total > 0 {
		p.Fail(w, 400, "该兑换码已有兑换记录，请停用以保留记录")
		return
	}
	code, err := a.find("codes", "id", r.PathValue("id"))
	if c.Err(w, err) {
		return
	}
	if p.Number(code, "historicalUses") > 0 {
		p.Fail(w, 400, "该兑换码已有历史用量，请停用以保留记录")
		return
	}
	if !c.Err(w, a.C.Delete("codes", r.PathValue("id"))) {
		p.JSON(w, true)
	}
}

func codeSummary(code p.Record) string {
	switch p.Text(code, "kind") {
	case "balance":
		return fmt.Sprintf("余额 %.2f", p.Number(code, "amount")/100)
	case "traffic":
		return fmt.Sprintf("追加 %g GB", p.Number(code, "quotaGiB"))
	case "reset":
		return "重置本周期流量"
	case "time":
		return "延长 " + codeDuration(code)
	default:
		return fmt.Sprintf("%g GB · %s", p.Number(code, "quotaGiB"), codeDuration(code))
	}
}

func codeDuration(code p.Record) string {
	if p.Text(code, "durationUnit") == "permanent" {
		return "长期"
	}
	unit := "天"
	if p.Text(code, "durationUnit") == "month" {
		unit = "个月"
	}
	return fmt.Sprintf("%g%s", p.Number(code, "durationValue"), unit)
}

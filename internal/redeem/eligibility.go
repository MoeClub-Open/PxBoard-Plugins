package redeem

import (
	"fmt"
	"slices"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func (a *app) checkEligibility(code p.Record, user string) error {
	if p.Bool(code, "reviewRequired") {
		return fmt.Errorf("兑换码奖励配置需要管理员核对")
	}
	if p.Number(code, "amount") > 0 && !a.walletAvailable() {
		return fmt.Errorf("本站尚未启用余额兑换")
	}
	if group := p.Text(code, "redemptionGroup"); group != "" {
		claims, err := a.all("claims", equal("redemptionGroup", group), equal("userId", user))
		if err != nil {
			return err
		}
		if p.Number(code, "groupPerUserLimit") > 0 && len(claims) >= int(p.Number(code, "groupPerUserLimit")) {
			return fmt.Errorf("已达到该组兑换码的领取次数")
		}
		for _, claim := range claims {
			created, _ := time.Parse(time.RFC3339, p.Text(claim, "createdAt"))
			if time.Since(created) < time.Duration(p.Number(code, "cooldownHours")*float64(time.Hour)) {
				return fmt.Errorf("尚未达到该组兑换码的下次领取时间")
			}
		}
	}
	if !p.Bool(code, "requiresPlan") && !p.Bool(code, "requiresInactiveAccount") && !p.Bool(code, "restrictPlanFamilies") {
		return nil
	}
	var account p.Record
	if err := a.C.Call("access", "GET", "/internal/accounts/"+user, nil, &account); err != nil {
		return err
	}
	return accountEligibility(code, account)
}

func accountEligibility(code, account p.Record) error {
	family := p.Text(account, "planFamilyId")
	if p.Bool(code, "requiresPlan") && family == "" {
		return fmt.Errorf("请先开通套餐再使用此兑换码")
	}
	expires, _ := time.Parse(time.RFC3339, p.Text(account, "expiresAt"))
	active := family != "" && p.Text(account, "status") == "active" && (p.Text(account, "expiresAt") == "" || expires.After(time.Now()))
	if p.Bool(code, "requiresInactiveAccount") && active {
		return fmt.Errorf("当前套餐仍可使用，暂不符合此兑换码的领取条件")
	}
	if p.Bool(code, "restrictPlanFamilies") && family != "" {
		allowed := []string{}
		switch values := code["allowedPlanFamilies"].(type) {
		case []any:
			for _, value := range values {
				allowed = append(allowed, fmt.Sprint(value))
			}
		case []string:
			allowed = values
		}
		if !slices.Contains(allowed, family) {
			return fmt.Errorf("当前套餐不符合此兑换码的领取条件")
		}
	}
	return nil
}

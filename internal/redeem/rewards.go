package redeem

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func rewardFor(code, claim p.Record) p.Record {
	operation := "redeem-" + p.Text(claim, "id")
	if p.Text(code, "kind") == "balance" {
		return p.Record{"kind": "balance", "amount": code["amount"], "operationId": operation, "reason": "兑换码：" + p.Text(code, "name")}
	}
	grant := p.Record{"userId": claim["userId"], "grantId": operation, "kind": code["kind"]}
	for _, field := range []string{"durationUnit", "durationValue", "quotaGiB", "unlimited", "groups", "accessLevel", "speedLimitMbps", "deviceLimit", "planFamilyId", "timezone", "resetPolicy"} {
		if value, ok := code[field]; ok {
			grant[field] = value
		}
	}
	grant["preserveUsage"] = code["preserveUsage"]
	reward := p.Record{"kind": "access", "grant": grant}
	if p.Number(code, "amount") > 0 {
		return p.Record{"kind": "bundle", "items": []p.Record{{"kind": "balance", "amount": code["amount"], "operationId": operation, "reason": "兑换码：" + p.Text(code, "name")}, reward}}
	}
	return reward
}

func validateReward(claim p.Record) error {
	if p.Text(claim, "status") == "redeemed" {
		return nil
	}
	if p.Text(claim, "status") != "pending" {
		return fmt.Errorf("兑换记录状态无效")
	}
	reward := object(claim["reward"])
	switch p.Text(reward, "kind") {
	case "bundle":
		items, ok := reward["items"].([]any)
		if !ok || len(items) == 0 {
			return fmt.Errorf("组合兑换缺少奖励内容")
		}
		for _, item := range items {
			if err := validateReward(p.Record{"status": "pending", "userId": claim["userId"], "reward": item}); err != nil {
				return err
			}
		}
	case "balance":
		if p.Number(reward, "amount") <= 0 || p.Text(reward, "operationId") == "" {
			return fmt.Errorf("待完成余额兑换缺少金额或操作编号")
		}
	case "access":
		grant := object(reward["grant"])
		if p.Text(grant, "userId") != p.Text(claim, "userId") || p.Text(grant, "grantId") == "" {
			return fmt.Errorf("待完成权益兑换缺少原始开通编号或所属用户")
		}
	default:
		return fmt.Errorf("待完成兑换缺少权益快照")
	}
	return nil
}

func (a *app) deliverReward(claim, reward p.Record) error {
	switch p.Text(reward, "kind") {
	case "bundle":
		switch items := reward["items"].(type) {
		case []any:
			for _, item := range items {
				if err := a.deliverReward(claim, object(item)); err != nil {
					return err
				}
			}
		case []p.Record:
			for _, item := range items {
				if err := a.deliverReward(claim, item); err != nil {
					return err
				}
			}
		}
		return nil
	case "balance":
		return a.C.Call("commerce", "POST", "/internal/wallet/credits", p.Record{"userId": claim["userId"], "amount": reward["amount"], "operationId": reward["operationId"], "reason": reward["reason"]}, nil)
	case "access":
		return a.C.Call("access", "POST", "/internal/grant", reward["grant"], nil)
	default:
		return fmt.Errorf("兑换奖励配置无效")
	}
}

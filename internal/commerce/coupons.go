package commerce

import (
	"fmt"
	"slices"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func stringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			out = append(out, fmt.Sprint(item))
		}
		return out
	}
	return nil
}

func checkCoupon(coupon, plan, period p.Record, user, now string, orders []p.Record) error {
	if !p.Bool(coupon, "enabled") || p.Text(coupon, "expiresAt") != "" && p.Text(coupon, "expiresAt") < now || p.Text(coupon, "startsAt") != "" && p.Text(coupon, "startsAt") > now {
		return fmt.Errorf("优惠码无效、尚未开始或已到期")
	}
	if allowed := stringList(coupon["planFamilyIds"]); len(allowed) > 0 && !slices.Contains(allowed, p.Text(plan, "planFamilyId")) {
		return fmt.Errorf("此优惠码不适用于所选套餐")
	}
	if allowed := stringList(coupon["periodIds"]); len(allowed) > 0 && !slices.Contains(allowed, p.Text(period, "id")) {
		return fmt.Errorf("此优惠码不适用于所选周期")
	}
	used, personal := 0, 0
	for _, order := range orders {
		if order["couponId"] == coupon["id"] && p.Text(order, "status") != "canceled" {
			used++
			if order["userId"] == user {
				personal++
			}
		}
	}
	if p.Number(coupon, "limit") > 0 && used >= int(p.Number(coupon, "limit")) {
		return fmt.Errorf("优惠码已用完")
	}
	if p.Number(coupon, "perUserLimit") > 0 && personal >= int(p.Number(coupon, "perUserLimit")) {
		return fmt.Errorf("已达到此优惠码的每账号使用次数")
	}
	return nil
}

func (a *app) planOptions() ([]p.Option, []p.Option, error) {
	plans, err := a.C.List("plans")
	if err != nil {
		return nil, nil, err
	}
	families, periods := []p.Option{}, []p.Option{}
	seen := map[string]bool{}
	for _, plan := range plans {
		family := p.Text(plan, "planFamilyId")
		if family == "" {
			family = p.Text(plan, "id")
		}
		if !seen[family] {
			families = append(families, p.Option{Value: family, Label: p.Text(plan, "name")})
			seen[family] = true
		}
		for _, period := range planPeriods(plan) {
			periods = append(periods, p.Option{Value: p.Text(period, "id"), Label: p.Text(plan, "name") + " · " + periodLabel(period)})
		}
	}
	return families, periods, nil
}

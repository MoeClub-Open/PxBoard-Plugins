package commerce

import (
	"fmt"
	"math"
	"net/http"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

// An old single-price plan is presented as one explicit choice until it is migrated.
func planPeriods(plan p.Record) []p.Record {
	if raw, exists := plan["periods"]; exists {
		var out []p.Record
		switch rows := raw.(type) {
		case []any:
			for _, row := range rows {
				if item, ok := row.(map[string]any); ok {
					out = append(out, item)
				}
			}
		case []p.Record:
			out = rows
		}
		return out
	}
	period := p.Record{"id": plan["id"], "label": periodLabel(plan)}
	for _, key := range []string{"price", "durationUnit", "durationValue", "kind", "resetPolicy"} {
		if value, ok := plan[key]; ok {
			period[key] = value
		}
	}
	return []p.Record{period}
}

func periodLabel(period p.Record) string {
	if name := p.Text(period, "label"); name != "" {
		return name
	}
	if p.Text(period, "kind") == "reset" {
		return "重置流量"
	}
	if p.Text(period, "kind") == "traffic" {
		return "流量加油包"
	}
	if p.Text(period, "durationUnit") == "permanent" {
		return "长期"
	}
	unit := "个月"
	if p.Text(period, "durationUnit") == "day" {
		unit = "天"
	}
	return fmt.Sprintf("%.0f %s", p.Number(period, "durationValue"), unit)
}

func periodPlan(plan, period p.Record) p.Record {
	out := p.Record{}
	for key, value := range plan {
		if key != "periods" {
			out[key] = value
		}
	}
	for _, key := range []string{"price", "durationUnit", "durationValue", "kind", "resetPolicy"} {
		if value, ok := period[key]; ok {
			out[key] = value
		}
	}
	return out
}

func periodAvailable(plan, period, account p.Record) bool {
	if p.Bool(plan, "reviewRequired") || p.Bool(period, "reviewRequired") {
		return false
	}
	if family := p.Text(plan, "planFamilyId"); family != "" && family == p.Text(account, "planFamilyId") {
		return plan["renewable"] != false && period["renewable"] != false
	}
	return p.Bool(plan, "enabled") && plan["visible"] != false && period["enabled"] != false && period["visible"] != false
}

func availablePeriods(plan, account p.Record) []p.Record {
	out := []p.Record{}
	for _, period := range planPeriods(plan) {
		if periodAvailable(plan, period, account) {
			out = append(out, period)
		}
	}
	return out
}

func validPrice(value any) bool {
	var amount float64
	switch v := value.(type) {
	case float64:
		amount = v
	case int:
		amount = float64(v)
	case int64:
		amount = float64(v)
	default:
		return false
	}
	return !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount >= 0 && math.Trunc(amount) == amount
}

func normalizePeriods(plan p.Record) error {
	if _, exists := plan["periods"]; !exists {
		return nil
	}
	periods := planPeriods(plan)
	if len(periods) == 0 {
		return fmt.Errorf("请至少设置一个购买周期")
	}
	seen := map[string]bool{}
	for _, item := range periods {
		if p.Text(item, "id") == "" {
			item["id"] = p.ID()
		}
		id := p.Text(item, "id")
		if seen[id] {
			return fmt.Errorf("购买周期不能重复")
		}
		seen[id] = true
		if p.Text(item, "label") == "" {
			item["label"] = periodLabel(item)
		}
		if !validPrice(item["price"]) {
			return fmt.Errorf("周期价格须为非负整数")
		}
		effective := periodPlan(plan, item)
		kind, unit, duration := p.Text(effective, "kind"), p.Text(effective, "durationUnit"), p.Number(effective, "durationValue")
		if !c.Allowed(kind, "subscription", "time", "traffic", "reset") || !c.Allowed(unit, "day", "month", "permanent") || (kind != "reset" && kind != "traffic" && unit != "permanent" && (duration < 1 || math.Trunc(duration) != duration)) {
			return fmt.Errorf("请设置有效的周期类型和期限")
		}
	}
	plan["periods"] = periods
	return nil
}

func (a *app) selectPeriod(planID, periodID string) (p.Record, p.Record, error) {
	var plans []p.Record
	if planID != "" {
		plan, err := a.Find("plans", planID)
		if err != nil {
			return nil, nil, err
		}
		plans = []p.Record{plan}
	} else if periodID != "" {
		var err error
		plans, err = a.C.List("plans")
		if err != nil {
			return nil, nil, err
		}
	} else {
		return nil, nil, fmt.Errorf("请选择套餐和购买周期")
	}
	var selected, choice p.Record
	for _, plan := range plans {
		periods := planPeriods(plan)
		if periodID == "" && len(periods) == 1 {
			periodID = p.Text(periods[0], "id")
		}
		for _, period := range periods {
			if p.Text(period, "id") != periodID || periodID == "" {
				continue
			}
			if selected != nil {
				return nil, nil, fmt.Errorf("周期标识重复，请联系管理员")
			}
			selected, choice = plan, period
		}
	}
	if selected == nil {
		return nil, nil, fmt.Errorf("请选择有效的购买周期")
	}
	if !validPrice(periodPlan(selected, choice)["price"]) {
		return nil, nil, fmt.Errorf("购买周期价格无效，请联系管理员")
	}
	return selected, choice, nil
}

func (a *app) periodOptions(w http.ResponseWriter, r *http.Request) {
	if !c.Member(w, r) {
		return
	}
	plans, err := a.C.List("plans")
	if c.Err(w, err) {
		return
	}
	account, err := a.purchaseAccount(p.Subject(r).ID)
	if c.Err(w, err) {
		return
	}
	options := []p.Record{}
	for _, plan := range plans {
		if id := r.PathValue("id"); id != "" && p.Text(plan, "id") != id {
			continue
		}
		for _, period := range planPeriods(plan) {
			if !p.Admin(r) && !periodAvailable(plan, period, account) {
				continue
			}
			options = append(options, p.Record{"id": period["id"], "name": p.Text(plan, "name") + " · " + periodLabel(period)})
		}
	}
	p.JSON(w, options)
}

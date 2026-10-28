package commerce

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func plusMonths(t time.Time, n int) time.Time {
	start := time.Date(t.Year(), t.Month()+time.Month(n), 1, t.Hour(), t.Minute(), t.Second(), 0, t.Location())
	last := time.Date(start.Year(), start.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	return time.Date(start.Year(), start.Month(), min(t.Day(), last), t.Hour(), t.Minute(), t.Second(), 0, t.Location())
}

func importedPeriod(order p.Record) int {
	key := p.Text(order, "periodId")
	return map[string]int{"monthly": 1, "month_price": 1, "quarterly": 3, "quarter_price": 3, "half_yearly": 6, "half_year_price": 6, "yearly": 12, "year_price": 12, "two_yearly": 24, "two_year_price": 24, "three_yearly": 36, "three_year_price": 36}[key]
}

func sourceRecord(row p.Record) p.Record {
	v, _ := row["sourceContent"].(map[string]any)
	return v
}

func normalizePaidTerms(orders []p.Record, receipts map[string]p.Record) {
	for _, order := range orders {
		if p.Bool(order, "imported") || p.Text(order, "coverageStartAt") != "" {
			continue
		}
		receipt := receipts[p.Text(order, "id")]
		if receipt == nil || p.Bool(receipt, "revoked") {
			continue
		}
		plan, _ := order["plan"].(map[string]any)
		if plan == nil || p.Text(plan, "kind") != "subscription" {
			continue
		}
		start, err := time.Parse(time.RFC3339, p.Text(receipt, "createdAt"))
		if err != nil {
			continue
		}
		if old, e := time.Parse(time.RFC3339, p.Text(receipt, "expiresAt")); e == nil && old.After(start) {
			start = old
		}
		end := start
		count := int(p.Number(plan, "durationValue"))
		switch p.Text(plan, "durationUnit") {
		case "month":
			end = plusMonths(start, count)
		case "day":
			end = start.AddDate(0, 0, count)
		}
		order["coverageStartAt"], order["coverageEndAt"] = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
		order["planFamilyId"], order["entitlementValue"] = plan["planFamilyId"], p.Number(order, "amount")
	}
}

func importedValue(order p.Record) float64 {
	raw := sourceRecord(order)
	amount := func(key string) float64 { value, _ := strconv.ParseFloat(fmt.Sprint(raw[key]), 64); return value }
	return max(0, p.Number(order, "amount")+p.Number(order, "balanceAmount")+amount("surplus_amount")-amount("surplus_credit")-p.Number(order, "refundAmount"))
}

// Paid terms retain their own coverage dates; gifted time and a later catalog
// price cannot increase the value. Imported Xboard/V2Board terms use their
// original order totals and period history, as the source panel does.
func remainingValue(account p.Record, orders []p.Record, now time.Time) (float64, []string) {
	ids := []string{}
	value := 0.0
	expiry, _ := time.Parse(time.RFC3339, p.Text(account, "expiresAt"))
	permanent := p.Text(account, "expiresAt") == ""
	legacyAmount, months := 0.0, 0
	var first time.Time
	legacyIDs := []string{}
	var lifetime p.Record
	for _, order := range orders {
		if p.Text(order, "status") != "fulfilled" || p.Text(order, "kind") != "subscription" {
			continue
		}
		if p.Bool(order, "imported") {
			raw := sourceRecord(order)
			planID := fmt.Sprint(raw["plan_id"])
			if planID == "" || planID != p.Text(account, "planSourceId") {
				continue
			}
			if permanent {
				if c := p.Text(order, "periodId"); c == "onetime" || c == "onetime_price" {
					if lifetime == nil || p.Text(order, "createdAt") > p.Text(lifetime, "createdAt") {
						lifetime = order
					}
				}
				continue
			}
			m := importedPeriod(order)
			if m == 0 {
				continue
			}
			created, err := time.Parse(time.RFC3339, p.Text(order, "createdAt"))
			if err != nil {
				continue
			}
			if first.IsZero() || created.Before(first) {
				first = created
			}
			legacyAmount += importedValue(order)
			months += m
			legacyIDs = append(legacyIDs, p.Text(order, "id"))
			continue
		}
		if p.Text(order, "planFamilyId") != p.Text(account, "planFamilyId") {
			continue
		}
		plan, _ := order["plan"].(map[string]any)
		if permanent {
			if p.Text(plan, "durationUnit") == "permanent" && (lifetime == nil || p.Text(order, "fulfilledAt") > p.Text(lifetime, "fulfilledAt")) {
				lifetime = order
			}
			continue
		}
		start, e1 := time.Parse(time.RFC3339, p.Text(order, "coverageStartAt"))
		end, e2 := time.Parse(time.RFC3339, p.Text(order, "coverageEndAt"))
		if e1 != nil || e2 != nil || !end.After(start) {
			continue
		}
		paid := p.Number(order, "entitlementValue")
		stop := end
		if !expiry.IsZero() && expiry.Before(stop) {
			stop = expiry
		}
		remaining := stop.Sub(maxTime(now, start)).Seconds()
		if remaining > 0 && paid > 0 {
			value += paid * min(1, remaining/end.Sub(start).Seconds())
			ids = append(ids, p.Text(order, "id"))
		}
	}
	if permanent && lifetime != nil {
		paid := p.Number(lifetime, "entitlementValue")
		if p.Bool(lifetime, "imported") {
			paid = importedValue(lifetime)
		}
		quota := p.Number(account, "quotaGiB")
		if plan, ok := lifetime["plan"].(map[string]any); ok && p.Number(plan, "quotaGiB") > 0 {
			quota = min(quota, p.Number(plan, "quotaGiB"))
		}
		ratio := 0.0
		if quota > 0 {
			ratio = max(0, min(1, (quota-p.Number(account, "usedGiB"))/quota))
		}
		if p.Bool(account, "unlimited") {
			ratio = 1
		}
		value = paid * ratio
		if value > 0 {
			ids = append(ids, p.Text(lifetime, "id"))
		}
	} else if months > 0 && !first.IsZero() {
		end := plusMonths(first, months)
		stop := end
		if !expiry.IsZero() && expiry.Before(stop) {
			stop = expiry
		}
		ratio := max(0, min(1, stop.Sub(now).Seconds()/end.Sub(first).Seconds()))
		if ratio > 0 {
			value += legacyAmount * ratio
			ids = append(ids, legacyIDs...)
		}
	}
	return math.Floor(value), ids
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func surplusIDs(row p.Record) []string {
	if ids, ok := row["surplusOrderIds"].([]string); ok {
		return ids
	}
	ids := []string{}
	if values, ok := row["surplusOrderIds"].([]any); ok {
		for _, v := range values {
			if id, ok := v.(string); ok && strings.TrimSpace(id) != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

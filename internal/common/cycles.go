package common

import (
	"fmt"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"strings"
	"time"
	_ "time/tzdata"
)

// NextReset calculates calendar boundaries in the source panel's timezone.
func NextReset(policy p.Record, after time.Time) (time.Time, error) {
	kind := p.Text(policy, "type")
	if kind == "never" || kind == "" {
		return time.Time{}, nil
	}
	zone, e := time.LoadLocation(p.Text(policy, "timezone"))
	if e != nil {
		return time.Time{}, e
	}
	local := after.In(zone)
	day := int(p.Number(policy, "day"))
	if day == 0 {
		day = 1
	}
	month := local.Month()
	year := local.Year()
	yearly := strings.HasPrefix(kind, "yearly")
	if yearly {
		month = time.Month(p.Number(policy, "month"))
		if month == 0 {
			month = 1
		}
	}
	if !strings.HasPrefix(kind, "monthly") && !yearly {
		return time.Time{}, fmt.Errorf("未知流量重置方式：%s", kind)
	}
	for {
		lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, zone).Day()
		if !yearly || !p.Bool(policy, "skipMissingYearDay") || day <= lastDay {
			next := time.Date(year, month, min(day, lastDay), int(p.Number(policy, "hour")), int(p.Number(policy, "minute")), int(p.Number(policy, "second")), 0, zone)
			if next.After(after) {
				return next.UTC(), nil
			}
		}
		if yearly {
			year++
		} else {
			month++
			if month > 12 {
				month = 1
				year++
			}
		}
	}
}

package migration

import (
	"testing"
)

func TestV2Routes(t *testing.T) {
	plan := &Plan{Mapped: map[string]int{}}
	m := &mapper{S: &Snapshot{Tables: map[string]Table{"v2_settings": {Rows: []Row{{"name": "secure_path", "value": "private"}, {"name": "subscribe_path", "value": "sub"}, {"name": "subscribe_url", "value": "https://one.example,https://two.example"}}}}}, Plan: plan}
	m.v2Routes()
	if len(plan.Errors) != 0 || len(plan.Rows) != 2 || plan.Rows[0].Kind != "setting" || plan.Rows[1].Collection != "public_routes" {
		t.Fatalf("route mapping failed: %+v", plan)
	}
	plan = &Plan{Mapped: map[string]int{}}
	m.Plan = plan
	m.S.Tables["v2_settings"] = Table{Rows: []Row{{"name": "subscribe_url", "value": "https://{random}.example"}}}
	m.v2Routes()
	if len(plan.Errors) == 0 {
		t.Fatal("dynamic address silently accepted")
	}
}

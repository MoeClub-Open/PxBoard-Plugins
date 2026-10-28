package migration

import (
	"encoding/json"
	"strings"
	"testing"

	sc "github.com/MoeClub-Open/PxBoard-Plugins/internal/subscriptionconfig"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestTemplateMigrationKeepsSourceAndSelectsMeta(t *testing.T) {
	plan := &Plan{Mapped: map[string]int{}}
	snapshot := &Snapshot{SourceID: "source", Tables: map[string]Table{"v2_subscribe_templates": {Rows: []Row{{"name": "clash", "content": strings.Replace(sc.Default("clash"), "MATCH,代理选择", "MATCH,missing", 1)}, {"name": "clashmeta", "content": sc.Default("clash")}, {"name": "singbox", "content": sc.Default("sing-box")}, {"name": "surge", "content": "unsupported-original"}}}}}
	mapper := &mapper{S: snapshot, Plan: plan, UsedTables: map[string]bool{}}
	mapper.v2Templates()
	if len(plan.Errors) > 0 {
		t.Fatal(plan.Errors)
	}
	foundSource, foundPolicy, foundDraft := false, false, false
	for _, row := range plan.Rows {
		var record p.Record
		if err := json.Unmarshal(row.Data, &record); err != nil {
			t.Fatal(err)
		}
		if row.ID == "source:clashmeta" {
			foundSource = p.Text(record, "originalContent") == sc.Default("clash")
		}
		if row.ID == "source:clash" {
			foundDraft = !p.Bool(record, "enabled") && p.Text(record, "originalContent") != "" && p.Text(record, "validationError") != ""
		}
		if row.Collection == "settings" {
			foundPolicy = p.Text(record, "clashTemplateId") == "source:clashmeta" && p.Text(record, "singboxTemplateId") == "source:singbox"
		}
	}
	if !foundSource || !foundPolicy || !foundDraft || len(plan.Warnings) == 0 {
		t.Fatal("template selection, source preservation or unsupported-format report missing")
	}
}

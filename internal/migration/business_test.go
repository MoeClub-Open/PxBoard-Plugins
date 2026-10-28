package migration

import (
	"encoding/json"
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestBusinessSupplementDoesNotReplayEntitlements(t *testing.T) {
	snapshot := &Snapshot{Source: "xboard", SourceID: "site", Timezone: "Asia/Shanghai", ExportedAt: "2026-10-28T00:00:00Z", Tables: map[string]Table{
		"v2_user":               {Rows: []Row{{"id": "1", "email": "member@example.test"}, {"id": "2", "email": "staff@example.test", "is_admin": "1"}}},
		"v2_plan":               {Rows: []Row{{"id": "3", "name": "套餐", "group_id": "1", "transfer_enable": "100", "reset_traffic_method": "2"}}},
		"v2_order":              {Rows: []Row{{"id": "5", "user_id": "1", "plan_id": "3", "period": "month_price", "trade_no": "original-order-number", "total_amount": "12345", "balance_amount": "100", "status": "3"}}},
		"v2_ticket":             {Rows: []Row{{"id": "4", "user_id": "1", "subject": "问题", "status": "0"}}},
		"v2_ticket_message":     {Rows: []Row{{"id": "2", "ticket_id": "4", "user_id": "2", "message": "![图](https://example.test/a.png)", "created_at": "200"}, {"id": "1", "ticket_id": "4", "user_id": "1", "message": "**问题**", "created_at": "100"}}},
		"v2_gift_card_template": {Rows: []Row{{"id": "1", "name": "组合兑换", "status": "1", "type": "1", "rewards": `{"balance":20000,"plan_id":3,"random_rewards":[]}`, "limits": `{"max_use_per_user":5}`}}},
		"v2_gift_card_code":     {Rows: []Row{{"id": "7", "template_id": "1", "code": "test-code", "max_usage": "1", "usage_count": "1", "status": "1"}}},
		"v2_gift_card_usage":    {Rows: []Row{{"id": "8", "code_id": "7", "template_id": "1", "user_id": "1", "rewards_given": `{"balance":20000,"plan_id":3}`}}},
	}}
	plan, err := Build(snapshot, "import-xboard", Options{Scope: "business", IncludeHistory: true, IncludeSupport: true, IncludeRedeem: true})
	if err != nil || len(plan.Errors) > 0 {
		t.Fatalf("%v %v", err, plan.Errors)
	}
	seen := map[string]bool{}
	for _, row := range plan.Rows {
		if row.Kind != "document" || row.Namespace == "access" || row.Collection == "ledger" || row.Collection == "archive" || row.Collection == "plans" {
			t.Fatalf("supplement would rewrite existing business state: %s/%s", row.Namespace, row.Collection)
		}
		var data p.Record
		if err := json.Unmarshal(row.Data, &data); err != nil {
			t.Fatal(err)
		}
		seen[row.Collection] = true
		switch row.Collection {
		case "orders":
			if data["number"] != "original-order-number" || data["amount"] != float64(12345) || data["status"] != "fulfilled" || data["imported"] != true {
				t.Fatal("order facts changed", data)
			}
		case "tickets":
			messages := data["messages"].([]any)
			if data["status"] != "answered" || len(messages) != 2 || messages[0].(map[string]any)["content"] != "**问题**" || messages[1].(map[string]any)["content"] != "![图](https://example.test/a.png)" {
				t.Fatal("ticket conversation or status changed")
			}
		case "codes":
			if data["amount"] != float64(20000) || data["kind"] != "access" || data["durationUnit"] != "unchanged" || data["groupPerUserLimit"] != float64(5) || data["historicalUses"] != float64(0) {
				t.Fatal("composite reward or shared usage limit lost")
			}
		case "claims":
			if data["status"] != "redeemed" || data["reward"] != nil || data["redemptionGroup"] != "site:gift-template:1" {
				t.Fatal("used reward could be delivered again")
			}
		}
	}
	if len(seen) != 4 {
		t.Fatal("missing business records", seen)
	}
}

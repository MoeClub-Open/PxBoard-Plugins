package redeem

import (
	"encoding/json"
	"testing"

	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestCompositeRewardHasStableOperationsAfterStorage(t *testing.T) {
	code := p.Record{"name": "组合兑换", "kind": "access", "amount": 20000, "durationUnit": "unchanged", "quotaGiB": 100, "groups": "group", "preserveUsage": true}
	claim := p.Record{"id": "claim", "userId": "member", "status": "pending"}
	claim["reward"] = rewardFor(code, claim)
	raw, _ := json.Marshal(claim)
	var saved p.Record
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if err := validateReward(saved); err != nil {
		t.Fatal(err)
	}
	items := object(saved["reward"])["items"].([]any)
	balance, access := object(items[0]), object(items[1])
	grant := object(access["grant"])
	if balance["operationId"] != "redeem-claim" || grant["grantId"] != "redeem-claim" || grant["preserveUsage"] != true || grant["durationUnit"] != "unchanged" {
		t.Fatal("reward cannot be retried without duplication")
	}
}

func TestPlanEligibilityKeepsAnEmptyAllowedList(t *testing.T) {
	code := p.Record{"requiresInactiveAccount": true, "restrictPlanFamilies": true, "allowedPlanFamilies": []any{}}
	if accountEligibility(code, p.Record{"effective": false, "planFamilyId": "existing", "status": "active"}) == nil {
		t.Fatal("active account received a restricted code")
	}
	if accountEligibility(code, p.Record{"effective": false, "planFamilyId": "existing"}) == nil {
		t.Fatal("empty allowed list was broadened to all plans")
	}
	if err := accountEligibility(code, p.Record{}); err != nil {
		t.Fatal("account without a plan should remain eligible", err)
	}
}

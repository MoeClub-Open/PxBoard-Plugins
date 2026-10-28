package access

import (
	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
	"net/http"
)

// Only billing can read the minimal grant receipts needed to value earlier
// purchases. Connection credentials and full previous account snapshots stay private.
func (a *app) billingReceipts(w http.ResponseWriter, r *http.Request) {
	if !c.Require(w, r, "commerce") {
		return
	}
	rows, err := a.recordsFor("accounts", "userId", r.PathValue("user"))
	if c.Err(w, err) {
		return
	}
	out := map[string]p.Record{}
	if len(rows) > 0 {
		grants, _ := rows[0]["grants"].(map[string]any)
		for id, raw := range grants {
			grant, _ := raw.(map[string]any)
			item := p.Record{}
			for _, key := range []string{"createdAt", "expiresAt", "revoked", "grantedExpiresAt"} {
				item[key] = grant[key]
			}
			out[id] = item
		}
	}
	p.JSON(w, out)
}

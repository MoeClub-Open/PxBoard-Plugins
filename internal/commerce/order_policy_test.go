package commerce

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	c "github.com/MoeClub-Open/PxBoard-Plugins/internal/common"
	p "github.com/MoeClub-Open/PxBoard/pkg/plugin"
)

func TestOrderExpiryPreservesPaidAndDebitedOrders(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, status string
		age          time.Duration
		debited      bool
		cancel       bool
	}{
		{"unpaid overdue", "pending", 30 * time.Minute, false, true},
		{"within payment window", "pending", 29 * time.Minute, false, false},
		{"wallet already deducted", "pending", time.Hour, true, false},
		{"paid awaiting fulfillment", "paid", time.Hour, false, false},
		{"completed", "fulfilled", time.Hour, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/runtime/query" {
					items := []p.Record{}
					if test.debited {
						items = append(items, p.Record{"id": "order-test"})
					}
					p.JSON(w, p.QueryResult{Items: items, Total: len(items)})
					return
				}
				if r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/api/v1/runtime/data/orders/") {
					var saved p.Record
					if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
						t.Error(err)
					}
					if saved["status"] != "canceled" || saved["cancelReason"] != "timeout" || saved["canceledAt"] != now.Format(time.RFC3339) {
						t.Error("cancellation metadata missing")
					}
					writes++
					p.JSON(w, true)
					return
				}
				t.Error("unexpected financial operation", r.URL.Path)
				http.Error(w, "unexpected", 500)
			}))
			defer server.Close()
			app := &app{&c.App{C: &p.Client{BaseURL: server.URL, HTTP: server.Client()}}}
			order := p.Record{"id": "test", "status": test.status, "createdAt": now.Add(-test.age).Format(time.RFC3339)}
			canceled, err := app.expirePending(order, paymentTimeout(p.Record{}), now)
			if err != nil || canceled != test.cancel || (writes > 0) != test.cancel {
				t.Fatalf("canceled=%v writes=%d err=%v", canceled, writes, err)
			}
		})
	}
}

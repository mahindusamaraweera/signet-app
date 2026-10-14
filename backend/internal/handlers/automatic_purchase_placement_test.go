package handlers

import (
	"database/sql"
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func TestAutomaticPurchaseAcceptsExistingTreePlacementKinds(t *testing.T) {
	for _, node := range []string{"deactive", "active", "gratitude", "correct"} {
		t.Run(node, func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
				{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"pending"}},
				{contains: "SELECT id,user_id,user_package_id", columns: []string{"id", "user_id", "user_package_id", "package_price", "purchase_kind", "status"}, values: []driver.Value{int64(1), int64(42), int64(9), "1000", "first", "awaiting_deposit"}},
				{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
				{contains: "node IN ('deactive','active','gratitude','correct')", columns: []string{"id"}, values: []driver.Value{int64(5)}},
				{contains: "UPDATE package_purchase_intents SET status="},
			}}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"status":"awaiting_deposit","available_usdt":"1130.99","remaining_usdt":"0","deposit_seen":true,"required_usdt":"1130","service_fee_usdt":"130"}`))
			}))
			defer server.Close()
			gateway, _ := blockchain.New(server.URL, "test")
			err := syncAutomaticPurchase(&app.Deps{DB: db, Blockchain: gateway}, 1)
			if err != nil || calls != 1 || !fixture.committed || len(fixture.steps) != 0 {
				t.Fatalf("placement %s prevented custody sync: %v calls=%d", node, err, calls)
			}
		})
	}
}

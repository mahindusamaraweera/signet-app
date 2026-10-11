package handlers

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func TestCommissionDeliveryRecordsOnlyValidatedCustodyState(t *testing.T) {
	for _, scenario := range []string{"authorized", "confirming", "confirmed", "missing confirmation", "wrong buyer", "wrong payment", "wrong recipient", "database failure"} {
		t.Run(scenario, func(t *testing.T) {
			address := "0x" + strings.Repeat("a", 40)
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "p.purchase_kind='first' FOR UPDATE",
				columns: []string{"id", "earn_log_id", "purchase_intent_id", "buyer_id", "beneficiary_user_id", "commission_kind", "commission_usd", "remote_payment_id", "recipient_address"},
				values:  []driver.Value{int64(7), int64(18), int64(1), int64(42), int64(77), "direct_team", "50", "9", address}}}}
			valid := scenario == "authorized" || scenario == "confirming" || scenario == "confirmed"
			if valid || scenario == "database failure" {
				step := commissionSQLStep{contains: "UPDATE blockchain_commission_outbox SET status=?"}
				if scenario == "database failure" {
					step.err = errors.New("write failed")
				}
				fixture.steps = append(fixture.steps, step)
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			body := map[string]interface{}{"id": "9", "external_user_id": "42", "external_purchase_id": "1", "external_outbox_id": "7", "recipient_address": address,
				"status": "authorized", "quote": map[string]string{"signet_price_usdt": "0.1", "signet_amount": "500"}}
			switch scenario {
			case "confirming":
				body["status"] = "confirming"
				body["tx_hash"] = "0x" + strings.Repeat("b", 64)
			case "confirmed":
				body["status"] = "confirmed"
				body["tx_hash"] = "0x" + strings.Repeat("b", 64)
				body["confirmed_at"] = "2026-10-10T00:00:00Z"
			case "missing confirmation":
				body["status"] = "confirmed"
			case "wrong buyer":
				body["external_user_id"] = "43"
			case "wrong payment":
				body["id"] = "10"
			case "wrong recipient":
				body["recipient_address"] = "0x" + strings.Repeat("c", 40)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/integration/commissions/authorize" || r.Header.Get("Idempotency-Key") != "signet-app:commission-authorize:7" {
					t.Error("wrong authorization request")
				}
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			gateway, _ := blockchain.New(server.URL, "test")
			err := syncCommissionDelivery(&app.Deps{DB: db, Blockchain: gateway}, 7)
			if (err == nil) != valid || fixture.committed != valid || len(fixture.steps) != 0 {
				t.Fatalf("invalid delivery transaction: err=%v committed=%v steps=%d", err, fixture.committed, len(fixture.steps))
			}
			if !valid && !fixture.rolledBack {
				t.Fatal("failed synchronization did not roll back")
			}
		})
	}
}

package handlers

import (
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestCompanySettlementWaitsForEvidenceAndPreservesRetryIdentity(t *testing.T) {
	for _, scenario := range []string{"unprepared", "sealed", "confirmed", "legacy", "network failure", "already confirmed"} {
		t.Run(scenario, func(t *testing.T) {
			state := "pending"
			if scenario == "already confirmed" {
				state = "confirmed"
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "status='completed' AND purchase_kind='first' FOR UPDATE", columns: []string{"id", "user_id", "package_price", "purchase_kind"}, values: []driver.Value{int64(1), int64(42), "100", "first"}},
				{contains: "INSERT INTO blockchain_company_settlements", args: []driver.Value{int64(1)}},
				{contains: "SELECT status FROM blockchain_company_settlements", columns: []string{"status"}, values: []driver.Value{state}},
			}}
			if scenario != "already confirmed" {
				commissionState := "confirmed"
				if scenario == "unprepared" {
					commissionState = "pending"
				}
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id", columns: []string{"id", "earn_log_id", "purchase_intent_id", "beneficiary_user_id", "commission_kind", "commission_usd", "status"}, values: []driver.Value{int64(7), int64(9), int64(1), int64(77), "direct_team", "75", commissionState}})
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if !fixture.committed {
					t.Error("custody contacted before accounting transaction committed")
				}
				var body struct {
					Buyer    string              `json:"external_user_id"`
					Purchase string              `json:"external_purchase_id"`
					Rows     []map[string]string `json:"commissions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if body.Buyer != "42" || body.Purchase != "1" || len(body.Rows) != 1 || body.Rows[0]["commission_usd"] != "75" {
					t.Error("source commission evidence changed")
				}
				if scenario == "network failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				raw, _ := json.Marshal(body.Rows)
				hash := fmt.Sprintf("%x", sha256.Sum256(raw))
				enabled, sealed, settled := true, true, scenario == "confirmed"
				if scenario == "legacy" {
					enabled, sealed, hash = false, false, ""
				}
				json.NewEncoder(w).Encode(blockchain.CompanySettlementResult{Enabled: &enabled, Sealed: &sealed, Settled: &settled, Hash: hash, Buyer: "42", Purchase: "1"})
			}))
			defer server.Close()
			if scenario == "sealed" || scenario == "confirmed" || scenario == "legacy" {
				expected := scenario
				if scenario == "legacy" {
					expected = "skipped"
				}
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "AND status IN ('pending','sealed')", checkArgs: func(args []driver.NamedValue) error {
					if args[0].Value != expected || args[2].Value != int64(1) {
						return fmt.Errorf("changed settlement state or purchase identity")
					}
					return nil
				}})
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			gateway, _ := blockchain.New(server.URL, "test")
			err := syncCompanySettlement(&app.Deps{DB: db, Blockchain: gateway}, 1)
			if (err != nil) != (scenario == "network failure") {
				t.Fatalf("unexpected result: %v", err)
			}
			if !fixture.committed || len(fixture.steps) != 0 {
				t.Fatal("source evidence or retry state not committed")
			}
			wantRequests := 1
			if scenario == "unprepared" || scenario == "already confirmed" {
				wantRequests = 0
			}
			if requests != wantRequests {
				t.Fatalf("got %d custody calls; want %d", requests, wantRequests)
			}
		})
	}
}

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
	"signet-backend/internal/config"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestCompanySettlementWaitsForEvidenceAndPreservesRetryIdentity(t *testing.T) {
	for _, scenario := range []string{"unprepared", "sealed", "confirmed", "legacy", "network failure", "already confirmed", "previously skipped"} {
		t.Run(scenario, func(t *testing.T) {
			state := "pending"
			if scenario == "already confirmed" {
				state = "confirmed"
			}
			if scenario == "previously skipped" {
				state = "skipped"
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
			if scenario == "sealed" || scenario == "confirmed" || scenario == "legacy" || scenario == "previously skipped" {
				expected := scenario
				if scenario == "previously skipped" {
					expected = "sealed"
				}
				if scenario == "legacy" {
					expected = "skipped"
				}
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "AND status IN ('pending','sealed','skipped')", checkArgs: func(args []driver.NamedValue) error {
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

func TestCompanySettlementExcludesOnlyCorroboratedCompanyAccounting(t *testing.T) {
	for _, scenario := range []string{"corroborated", "unmatched", "invalid marker"} {
		t.Run(scenario, func(t *testing.T) {
			columns := []string{"id", "earn_log_id", "purchase_intent_id", "beneficiary_user_id", "commission_kind", "commission_usd", "status"}
			paid := []driver.Value{int64(7), int64(9), int64(1), int64(77), "direct_team", "75", "confirmed"}
			extra := []driver.Value{int64(8), int64(10), int64(1), int64(1), "direct_team", "75", "requires_reconciliation"}
			if scenario == "invalid marker" {
				extra[3] = int64(78)
				extra[6] = "accounting_only"
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "status='completed' AND purchase_kind='first' FOR UPDATE", columns: []string{"id", "user_id", "package_price", "purchase_kind"}, values: []driver.Value{int64(1), int64(42), "100", "first"}},
				{contains: "INSERT INTO blockchain_company_settlements", args: []driver.Value{int64(1)}},
				{contains: "SELECT status FROM blockchain_company_settlements", columns: []string{"status"}, values: []driver.Value{"pending"}},
				{contains: "FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id", columns: columns, rowValues: [][]driver.Value{paid, extra}},
			}}
			if scenario != "invalid marker" {
				reloaded := append([]driver.Value(nil), extra...)
				if scenario == "corroborated" {
					reloaded[6] = "accounting_only"
				}
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "UPDATE blockchain_commission_outbox extra", args: []driver.Value{int64(1)}},
					commissionSQLStep{contains: "FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id", columns: columns, rowValues: [][]driver.Value{paid, reloaded}},
				)
			}
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Rows []map[string]string `json:"commissions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if len(body.Rows) != 1 || body.Rows[0]["external_outbox_id"] != "7" || body.Rows[0]["commission_usd"] != "75" {
					t.Error("secondary company entry authorized a duplicate payout")
				}
				raw, _ := json.Marshal(body.Rows)
				hash := fmt.Sprintf("%x", sha256.Sum256(raw))
				enabled, sealed, settled := true, true, false
				json.NewEncoder(w).Encode(blockchain.CompanySettlementResult{Enabled: &enabled, Sealed: &sealed, Settled: &settled, Hash: hash, Buyer: "42", Purchase: "1"})
			}))
			defer server.Close()
			if scenario == "corroborated" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "AND status IN ('pending','sealed','skipped')"})
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			g, _ := blockchain.New(server.URL, "test")
			err := syncCompanySettlement(&app.Deps{DB: db, Blockchain: g}, 1)
			if scenario == "invalid marker" {
				if err == nil || fixture.committed || calls != 0 {
					t.Fatalf("non-company entitlement was suppressed: %v", err)
				}
			} else if err != nil || !fixture.committed {
				t.Fatalf("accounting evidence failed: %v", err)
			}
			if scenario == "corroborated" && calls != 1 {
				t.Fatal("valid company remainder was blocked")
			}
			if scenario == "unmatched" && calls != 0 {
				t.Fatal("unmatched disputed row authorized settlement")
			}
			if len(fixture.steps) != 0 {
				t.Fatal("required accounting checks were omitted")
			}
		})
	}
}

func TestRecovered460CompanySettlementAcknowledgesReceiptWithoutManifestOrCommissions(t *testing.T) {
	for _, settled := range []bool{true, false} {
		fixture := &commissionSQLFixture{steps: []commissionSQLStep{
			{contains: "status='completed' AND purchase_kind='first' FOR UPDATE", columns: []string{"id", "user_id", "package_price", "purchase_kind"}, values: []driver.Value{int64(1), int64(460), "1000", "first"}},
			{contains: "INSERT INTO blockchain_company_settlements", args: []driver.Value{int64(1)}},
			{contains: "SELECT status FROM blockchain_company_settlements", columns: []string{"status"}, values: []driver.Value{"pending"}},
		}}
		if settled {
			fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SET status='confirmed',manifest_hash=?", args: []driver.Value{strings.Repeat("a", 64)}})
		}
		db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if !fixture.committed || r.URL.Path != "/api/v1/integration/package-purchases/company-status" {
				t.Fatal("financial write route called or source lock not released")
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"external_user_id": "460", "external_purchase_id": "1", "native_order_id": 5, "package_price_usdt": "1000", "sealed": true, "settled": settled, "manifest_hash": strings.Repeat("a", 64), "settlement_tx_hash": "0x" + strings.Repeat("b", 64), "settlement_usdt": "270.478", "settlement_confirmed_at": "2026-10-15 17:03:25"})
		}))
		gateway, _ := blockchain.New(server.URL, "test")
		err := syncCompanySettlement(&app.Deps{DB: db, Blockchain: gateway, Cfg: &config.Config{AppEnv: "production"}}, 1)
		server.Close()
		db.Close()
		if err != nil || requests != 1 || len(fixture.steps) != 0 {
			t.Fatalf("receipt reconciliation failed: %v requests=%d remaining=%d", err, requests, len(fixture.steps))
		}
	}
}

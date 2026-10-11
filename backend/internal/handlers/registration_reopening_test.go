package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func TestReopeningConfirmationUnlocksFundingAtomically(t *testing.T) {
	for _, scenario := range []string{"success", "bad confirmation", "confirmation write failure"} {
		badConfirmation := scenario == "bad confirmation"
		shouldFail := scenario != "success"
		fixture := &commissionSQLFixture{steps: []commissionSQLStep{
			{contains: "SELECT user_id FROM registration_reopening_outbox", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
			{contains: "SELECT id FROM users", columns: []string{"id"}, values: []driver.Value{int64(42)}},
			{contains: "SELECT user_id,expired_placement_log_id", columns: []string{"user_id", "expired_placement_log_id", "new_placement_log_id", "status"}, values: []driver.Value{int64(42), int64(7), int64(8), "pending"}},
			{contains: "SELECT user_id,status FROM package_purchase_intents", columns: []string{"user_id", "status"}, values: []driver.Value{int64(42), "awaiting_reopening"}},
			{contains: "SELECT user_id FROM user_parent_map_logs", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
			{contains: "SELECT id FROM user_parents", columns: []string{"id"}, values: []driver.Value{int64(55)}},
			{contains: "SELECT user_id FROM registration_expiries", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
		}}
		if !badConfirmation {
			fixture.steps = append(fixture.steps, commissionSQLStep{contains: "UPDATE package_purchase_intents SET status='awaiting_deposit'"}, commissionSQLStep{contains: "UPDATE registration_reopening_outbox SET status='confirmed'"})
			if scenario == "confirmation write failure" {
				fixture.steps[len(fixture.steps)-1].err = errors.New("confirmation write failed")
			}
		}
		db := sqlx.NewDb(sql.OpenDB(fixture), "mysql")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if badConfirmation {
				w.Write([]byte(`{"reopened":true}`))
				return
			}
			w.Write([]byte(`{"reopened":true,"external_user_id":"42","expired_placement_log_id":"7","new_placement_log_id":"8","external_purchase_id":"2"}`))
		}))
		gateway, _ := blockchain.New(server.URL, "test")
		err := syncRegistrationReopening(&app.Deps{DB: db, Blockchain: gateway}, 2)
		server.Close()
		db.Close()
		if (err != nil) != shouldFail || fixture.committed == shouldFail || fixture.rolledBack != shouldFail || len(fixture.steps) != 0 {
			t.Fatalf("unsafe funding unlock: %v", err)
		}
	}
}

package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWithdrawalRequestStatusIsOwnerScopedAndExcludesSecrets(t *testing.T) {
	for _, scenario := range []string{"queued", "expired", "wrong owner"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			owner := uint64(42)
			if scenario == "wrong owner" {
				owner = 43
			}
			terms, _ := NewWithdrawalTerms(owner, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			encoded, _ := json.Marshal(terms)
			status := "consumed"
			var queue driver.Value = "submitted"
			var remote driver.Value = int64(7)
			if scenario == "expired" {
				status = "authorized"
				queue = nil
				remote = nil
			}
			fixture := &sqlFixture{steps: []sqlStep{{match: "WHERE a.user_id=?", values: []driver.Value{strings.Repeat("c", 64), string(encoded), status, now.Add(-time.Second), now.Add(-time.Minute), queue, remote}, check: func(args []driver.NamedValue) error {
				if len(args) != 1 || args[0].Value != int64(42) {
					return errors.New("wrong session owner")
				}
				return nil
			}}}}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, _ := NewService(db, []byte(strings.Repeat("k", 32)))
			rows, err := s.ListWithdrawalRequests(context.Background(), 42, now)
			if scenario == "wrong owner" {
				if err == nil || rows != nil {
					t.Fatal("foreign request returned")
				}
				return
			}
			if err != nil || len(rows) != 1 {
				t.Fatalf("invalid status: %v", err)
			}
			if scenario == "expired" && rows[0].AuthorizationStatus != "expired" {
				t.Fatal("expired proof shown usable")
			}
			if scenario == "queued" && (rows[0].RemoteWithdrawalID == nil || *rows[0].RemoteWithdrawalID != "7" || rows[0].AuthorizationStatus != "consumed") {
				t.Fatal("queued financial record lost on expiry")
			}
			public, _ := json.Marshal(rows)
			for _, secret := range []string{"proof_hash", "credential_revision", "signature", "password", "last_error"} {
				if strings.Contains(string(public), secret) {
					t.Fatal("private authority exposed")
				}
			}
			if fixture.committed || fixture.rolledBack || len(fixture.steps) != 0 {
				t.Fatal("status read mutated data")
			}
		})
	}
}

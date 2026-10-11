package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"signet-backend/internal/auth"
	"strings"
	"testing"
	"time"
)

func TestCancellationIntentRequiresPasswordAndCommitsWithAudit(t *testing.T) {
	hash, _ := auth.HashPassword("test-password")
	for _, scenario := range []string{"draft", "queued", "audit failure", "wrong password"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			a, _, _ := NewWithdrawalAuthorization(key, terms, now)
			encoded, _ := json.Marshal(terms)
			fixture := &sqlFixture{steps: []sqlStep{{match: "SELECT password FROM users", values: []driver.Value{hash}}}}
			password := "test-password"
			if scenario == "wrong password" {
				password = "wrong"
			} else {
				status := "consumed"
				var approved driver.Value = now
				var consumed driver.Value = now
				if scenario == "draft" {
					status = "pending"
					approved = nil
					consumed = nil
				}
				fixture.steps = append(fixture.steps, sqlStep{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, status, a.ExpiresAt, approved, consumed}})
				if scenario == "draft" {
					fixture.steps = append(fixture.steps, sqlStep{match: "SET status='invalidated'"})
				} else {
					fixture.steps = append(fixture.steps, sqlStep{match: "FROM withdrawal_submission_outbox", values: []driver.Value{string(encoded), terms.Digest(), "pending"}}, sqlStep{match: "SET status='cancel_requested'"})
				}
				audit := sqlStep{match: "INSERT INTO withdrawal_security_audit"}
				if scenario == "audit failure" {
					audit.err = errors.New("audit failed")
				}
				fixture.steps = append(fixture.steps, audit)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, _ := NewService(db, key)
			err := s.RequestWithdrawalCancellation(context.Background(), 42, password, a.ID, now)
			success := scenario == "draft" || scenario == "queued"
			if (err == nil) != success || fixture.committed != success || len(fixture.steps) != 0 {
				t.Fatalf("wrong cancellation intent boundary: %v", err)
			}
		})
	}
}

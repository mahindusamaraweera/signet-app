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

func TestRemoteReservationBindingCommitsWithAudit(t *testing.T) {
	for _, scenario := range []string{"success", "retry", "changed remote", "audit failure", "cancel race"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			a, _, _ := NewWithdrawalAuthorization([]byte(strings.Repeat("k", 32)), terms, now)
			encoded, _ := json.Marshal(terms)
			status := "pending"
			var remote driver.Value
			if scenario == "retry" || scenario == "changed remote" {
				status = "submitted"
				remote = int64(7)
			}
			if scenario == "cancel race" {
				status = "cancel_requested"
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT id FROM users", values: []driver.Value{int64(42)}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "consumed", a.ExpiresAt, now, now}},
				{match: "FROM withdrawal_submission_outbox", values: []driver.Value{string(encoded), terms.Digest(), status, remote}},
			}}
			if status == "pending" || scenario == "cancel race" {
				fixture.steps = append(fixture.steps, sqlStep{match: "UPDATE withdrawal_submission_outbox", check: func(args []driver.NamedValue) error {
					expectedStatus := "submitted"
					if scenario == "cancel race" {
						expectedStatus = "cancel_requested"
					}
					if args[0].Value != expectedStatus {
						return errors.New("in-flight reservation erased cancellation intent")
					}
					return nil
				}}, sqlStep{match: "UPDATE withdrawal_authorizations"})
				audit := sqlStep{match: "INSERT INTO withdrawal_security_audit"}
				if scenario == "audit failure" {
					audit.err = errors.New("audit unavailable")
				}
				fixture.steps = append(fixture.steps, audit)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, _ := NewService(db, []byte(strings.Repeat("k", 32)))
			remoteID := uint64(7)
			if scenario == "changed remote" {
				remoteID = 8
			}
			err := s.RecordWithdrawalReservation(context.Background(), a.ID, terms, remoteID, now)
			success := scenario == "success" || scenario == "retry" || scenario == "cancel race"
			if (err == nil) != success || fixture.committed != success || len(fixture.steps) != 0 {
				t.Fatalf("wrong remote/audit boundary: %v", err)
			}
		})
	}
}

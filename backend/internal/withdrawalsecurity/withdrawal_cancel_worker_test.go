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

func TestRemoteCancellationRequiresExactIdentityAndAtomicAudit(t *testing.T) {
	for _, scenario := range []string{"tombstone", "recovered refund", "known remote lost", "audit failure", "retry"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			encoded, _ := json.Marshal(terms)
			a, _, _ := NewWithdrawalAuthorization(key, terms, now)
			status := "cancel_requested"
			var stored driver.Value
			var remote *uint64
			if scenario == "recovered refund" {
				v := uint64(7)
				remote = &v
			}
			if scenario == "known remote lost" {
				stored = int64(7)
			}
			if scenario == "retry" {
				status = "cancelled"
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT id FROM users", values: []driver.Value{int64(42)}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "consumed", a.ExpiresAt, now, now}},
				{match: "FROM withdrawal_submission_outbox", values: []driver.Value{string(encoded), terms.Digest(), status, stored}},
			}}
			if scenario != "known remote lost" && scenario != "retry" {
				fixture.steps = append(fixture.steps, sqlStep{match: "SELECT remote_withdrawal_id", values: []driver.Value{nil}}, sqlStep{match: "SET status='cancelled'"})
				if remote != nil {
					fixture.steps = append(fixture.steps, sqlStep{match: "UPDATE withdrawal_authorizations"})
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
			err := s.RecordWithdrawalCancellation(context.Background(), a.ID, terms, remote, now)
			success := scenario == "tombstone" || scenario == "recovered refund" || scenario == "retry"
			if (err == nil) != success || fixture.committed != success || len(fixture.steps) != 0 {
				t.Fatalf("wrong cancellation record boundary: %v", err)
			}
		})
	}
}

func TestUnsafeCancellationReviewPreservesLiabilityAndRequiresAudit(t *testing.T) {
	for _, failAudit := range []bool{false, true} {
		key := []byte(strings.Repeat("k", 32))
		now := time.Unix(1800000000, 0).UTC()
		terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
		encoded, _ := json.Marshal(terms)
		a, _, _ := NewWithdrawalAuthorization(key, terms, now)
		audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
			if args[1].Value != "withdrawal_cancel_review" {
				return errors.New("wrong review audit")
			}
			return nil
		}}
		if failAudit {
			audit.err = errors.New("audit failed")
		}
		fixture := &sqlFixture{steps: []sqlStep{
			{match: "SELECT id FROM users", values: []driver.Value{int64(42)}},
			{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "consumed", a.ExpiresAt, now, now}},
			{match: "FROM withdrawal_submission_outbox", values: []driver.Value{string(encoded), terms.Digest(), "cancel_requested", int64(7)}},
			{match: "SET status='cancellation_review_required'"}, audit,
		}}
		db := sql.OpenDB(fixture)
		s, _ := NewService(db, key)
		err := s.RecordWithdrawalCancellationReview(context.Background(), a.ID, terms, now)
		db.Close()
		if (err == nil) == failAudit || fixture.committed == failAudit || len(fixture.steps) != 0 {
			t.Fatalf("wrong review/audit boundary: %v", err)
		}
	}
}

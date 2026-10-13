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

	"signet-backend/internal/auth"
)

func TestWithdrawalQueueConsumptionAndOutboxCommitTogether(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "outbox failure", "queue audit failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			if err != nil {
				t.Fatal(err)
			}
			a, proof, err := NewWithdrawalAuthorization(key, terms, now)
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(terms)
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, int64(2), nil}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
				{match: "FROM withdrawal_security_addresses", values: []driver.Value{terms.Destination, now.Add(-24 * time.Hour), now, nil}},
				{match: "SELECT password FROM users", values: []driver.Value{password}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "authorized", a.ExpiresAt, now, nil}},
				{match: "SET status='consumed'"}, {match: "INSERT INTO withdrawal_security_audit"},
			}}
			outbox := sqlStep{match: "INSERT INTO withdrawal_submission_outbox", check: func(args []driver.NamedValue) error {
				if args[0].Value != a.ID || args[1].Value != int64(42) || args[2].Value != int64(2) || args[3].Value != string(encoded) || args[4].Value != terms.Digest() || args[7].Value != withdrawalCredentialDigest(key, 42, password) {
					return errors.New("wrong outbox terms")
				}
				return nil
			}}
			if scenario == "outbox failure" {
				outbox.err = errors.New("outbox failed")
			}
			fixture.steps = append(fixture.steps, outbox)
			if scenario != "outbox failure" {
				audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
					if args[1].Value != "withdrawal_queued" {
						return errors.New("wrong queue audit")
					}
					return nil
				}}
				if scenario == "queue audit failure" {
					audit.err = errors.New("queue audit failed")
				}
				fixture.steps = append(fixture.steps, audit)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			err = s.QueueWithdrawal(context.Background(), 42, "test-password", a.ID, proof, terms, now)
			commit := scenario == "success"
			if (err == nil) != commit || fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatalf("wrong outbox boundary: %v", err)
			}
		})
	}
}

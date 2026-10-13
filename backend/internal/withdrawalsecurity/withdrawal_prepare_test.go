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

func TestWithdrawalPreparationRejectsFrozenHeldOrUnverifiedAccounts(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"frozen", "hold", "email changed", "authenticator unconfirmed"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			verified := "user@example.com"
			frozen := false
			var hold driver.Value
			var confirmed driver.Value = now
			if scenario == "frozen" {
				frozen = true
			}
			if scenario == "hold" {
				hold = now.Add(time.Hour)
			}
			if scenario == "email changed" {
				verified = "old@example.com"
			}
			if scenario == "authenticator unconfirmed" {
				confirmed = nil
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{verified, now, nil, confirmed, frozen, int64(2), hold}},
			}}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, []byte(strings.Repeat("k", 32)))
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := s.PrepareWithdrawal(context.Background(), 42, "test-password", strings.Repeat("a", 64), "100", now)
			if err == nil || prepared.ID != "" || prepared.Proof != "" || fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
				t.Fatal("ineligible preparation persisted or returned proof")
			}
		})
	}
}

func TestWithdrawalPreparationStoresCanonicalTermsAndRollsBackFailures(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "revoked address", "address wait", "quota", "insert failure", "audit failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			id := strings.Repeat("a", 64)
			destination := "0x" + strings.Repeat("b", 40)
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			eligible := now
			var revoked driver.Value
			if scenario == "address wait" {
				eligible = now.Add(time.Second)
			}
			if scenario == "revoked address" {
				revoked = now
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, int64(2), now}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
				{match: "FROM withdrawal_security_addresses", values: []driver.Value{destination, now.Add(-24 * time.Hour), eligible, revoked}, check: func(args []driver.NamedValue) error {
					if args[0].Value != id || args[1].Value != int64(42) {
						return errors.New("wrong owner/address")
					}
					return nil
				}},
			}}
			var stored WithdrawalAuthorization
			if scenario != "address wait" && scenario != "revoked address" {
				count := int64(0)
				if scenario == "quota" {
					count = 5
				}
				fixture.steps = append(fixture.steps, sqlStep{match: "SELECT COUNT(*) FROM withdrawal_authorizations", values: []driver.Value{count}})
				if count == 0 {
					insert := sqlStep{match: "INSERT INTO withdrawal_authorizations", check: func(args []driver.NamedValue) error {
						var terms WithdrawalTerms
						if err := json.Unmarshal([]byte(args[4].Value.(string)), &terms); err != nil {
							return err
						}
						if terms.Net != "97.000000000000000000" || terms.Fee != "3.000000000000000000" || terms.UserID != 42 || terms.SecurityVersion != 2 || args[5].Value != terms.Digest() {
							return errors.New("wrong stored terms")
						}
						stored = WithdrawalAuthorization{ID: args[0].Value.(string), Terms: terms, ProofHash: args[6].Value.(string), Status: "pending", ExpiresAt: args[7].Value.(time.Time)}
						return nil
					}}
					if scenario == "insert failure" {
						insert.err = errors.New("insert failed")
					}
					fixture.steps = append(fixture.steps, insert)
					if scenario != "insert failure" {
						audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
							if args[0].Value != int64(42) || args[1].Value != "withdrawal_prepared" || args[2].Value != stored.ID {
								return errors.New("wrong audit")
							}
							return nil
						}}
						if scenario == "audit failure" {
							audit.err = errors.New("audit failed")
						}
						fixture.steps = append(fixture.steps, audit)
					}
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := s.PrepareWithdrawal(context.Background(), 42, "test-password", id, "100", now)
			if scenario == "success" {
				if err != nil || prepared.ID != stored.ID || stored.ProofHash != stored.hash(key, prepared.Proof) || prepared.Proof == stored.ProofHash || prepared.ExpiresAt != now.Add(10*time.Minute) {
					t.Fatalf("invalid prepared proof: %v", err)
				}
			} else if err == nil || prepared.ID != "" || prepared.Proof != "" {
				t.Fatal("failed preparation leaked proof")
			}
			commit := scenario == "success"
			if fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatal("wrong preparation transaction boundary")
			}
		})
	}
}

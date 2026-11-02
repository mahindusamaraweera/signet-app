package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/auth"
)

func TestAuthenticatorConfirmationTransactionBoundaries(t *testing.T) {
	passwordHash, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "success short hold", "wrong code", "changed version", "expired", "profile failure", "audit failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(59, 0).UTC()
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			version := int64(1)
			expires := now.Add(10 * time.Minute)
			hold := now.Add(72 * time.Hour)
			expectedHold := hold
			if scenario == "success short hold" {
				hold = now.Add(time.Hour)
				expectedHold = now.Add(24 * time.Hour)
			}
			if scenario == "changed version" {
				version = 2
			}
			if scenario == "expired" {
				expires = now
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", passwordHash, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, nil, false, version, hold}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), int64(0), expires, nil}},
			}}
			if scenario != "changed version" && scenario != "expired" {
				fixture.steps = append(fixture.steps, sqlStep{match: "SET attempts=attempts+1"})
				if scenario != "wrong code" {
					fixture.steps = append(fixture.steps, sqlStep{match: "INSERT INTO withdrawal_totp_counters"}, sqlStep{match: "SET last_counter = ?"})
					profile := sqlStep{match: "SET authenticator_fingerprint=?", check: func(args []driver.NamedValue) error {
						if args[0].Value != secretFingerprint(key, secret) || args[2].Value != expectedHold {
							return errors.New("fingerprint or longer hold changed")
						}
						return nil
					}}
					if scenario == "profile failure" {
						profile.err = errors.New("profile update failed")
					}
					fixture.steps = append(fixture.steps, profile)
					if scenario == "success" || scenario == "success short hold" || scenario == "audit failure" {
						fixture.steps = append(fixture.steps, sqlStep{match: "SET confirmed_at=?"})
						audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
							if args[0].Value != int64(42) || args[1].Value != "authenticator_confirmed" || args[2].Value != securityChangeReference(42, 2, "authenticator_confirmed") || args[3].Value != int64(2) {
								return errors.New("wrong authenticator audit attribution")
							}
							return nil
						}}
						if scenario == "audit failure" {
							audit.err = errors.New("audit storage failed")
						}
						fixture.steps = append(fixture.steps, audit)
					}
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			service, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			code := "287082"
			if scenario == "wrong code" {
				code = "000000"
			}
			err = service.ConfirmAuthenticator(context.Background(), 42, "test-password", code, now)
			if scenario == "success" || scenario == "success short hold" {
				if err != nil || !fixture.committed || fixture.rolledBack {
					t.Fatalf("confirmation failed: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid confirmation accepted")
				}
				commit := scenario == "wrong code"
				if fixture.committed != commit || fixture.rolledBack == commit {
					t.Fatal("wrong transaction boundary")
				}
			}
			if len(fixture.steps) != 0 {
				t.Fatalf("%d SQL steps not executed", len(fixture.steps))
			}
		})
	}
}

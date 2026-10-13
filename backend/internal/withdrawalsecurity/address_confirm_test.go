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

func TestAddressConfirmationCommitsFailedProofAttemptsWithoutEnrollment(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"wrong email code", "wrong authenticator", "success", "existing address", "insert failure", "consume failure", "profile failure", "audit failure", "revoke success"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(59, 0).UTC()
			address := "0x" + strings.Repeat("a", 40)
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			c, emailCode, err := NewAddressEnrollmentChallenge(key, 42, 2, "user@example.com", address, now)
			if scenario == "revoke success" {
				address = strings.Repeat("a", 64)
				c, emailCode, err = NewAddressRevocationChallenge(key, 42, 2, "user@example.com", address, now)
			}
			if err != nil {
				t.Fatal(err)
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{c.Email, password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{c.Email, now, secretFingerprint(key, secret), now, false, int64(2)}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), c.Email, int64(1), now}},
				{match: "FROM withdrawal_security_challenges", values: []driver.Value{c.ID, int64(42), c.Purpose, c.Target, c.Email, c.Hash, int64(2), "delivered", int64(0), c.ExpiresAt, nil}},
				{match: "SET attempts=attempts+1"},
			}}
			if scenario == "wrong email code" {
				emailCode = "invalid"
			}
			authenticatorCode := "invalid"
			if scenario != "wrong email code" && scenario != "wrong authenticator" {
				authenticatorCode = "287082"
				fixture.steps = append(fixture.steps, sqlStep{match: "INSERT INTO withdrawal_totp_counters"}, sqlStep{match: "SET last_counter = ?"})
				if scenario == "revoke success" {
					fixture.steps = append(fixture.steps, sqlStep{match: "SELECT revoked_at", values: []driver.Value{nil}}, sqlStep{match: "SET revoked_at=?"}, sqlStep{match: "SET status='consumed'"}, sqlStep{match: "SET security_version=security_version+1"}, sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
						if args[1].Value != "address_revoked" {
							return errors.New("wrong revocation audit")
						}
						return nil
					}})
				} else {
					existing := sqlStep{match: "FROM withdrawal_security_addresses", err: sql.ErrNoRows}
					if scenario == "existing address" {
						existing.err = nil
						existing.values = []driver.Value{strings.Repeat("b", 64)}
					}
					fixture.steps = append(fixture.steps, existing)
					if scenario != "existing address" {
						insert := sqlStep{match: "INSERT INTO withdrawal_security_addresses", check: func(args []driver.NamedValue) error {
							if !challengeIDPattern.MatchString(args[0].Value.(string)) || args[1].Value != int64(42) || args[2].Value != address || args[3].Value != int64(3) || args[4].Value != now || args[5].Value != now.Add(24*time.Hour) {
								return errors.New("wrong verified address terms")
							}
							return nil
						}}
						if scenario == "insert failure" {
							insert.err = errors.New("insert failed")
						}
						fixture.steps = append(fixture.steps, insert)
						if scenario != "insert failure" {
							consume := sqlStep{match: "SET status='consumed'"}
							if scenario == "consume failure" {
								consume.err = errors.New("consumption failed")
							}
							fixture.steps = append(fixture.steps, consume)
							if scenario != "consume failure" {
								profile := sqlStep{match: "SET security_version=security_version+1"}
								if scenario == "profile failure" {
									profile.err = errors.New("profile failed")
								}
								fixture.steps = append(fixture.steps, profile)
								if scenario != "profile failure" {
									audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
										if args[0].Value != int64(42) || args[1].Value != "address_verified" || args[2].Value != c.ID || args[3].Value != int64(3) {
											return errors.New("wrong audit attribution")
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
					}
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "revoke success" {
				err = s.ConfirmAddressRevocation(context.Background(), 42, "test-password", address, c.ID, emailCode, authenticatorCode, now)
			} else {
				err = s.ConfirmAddress(context.Background(), 42, "test-password", address, c.ID, emailCode, authenticatorCode, now)
			}
			commit := scenario == "success" || scenario == "revoke success" || scenario == "wrong email code" || scenario == "wrong authenticator"
			if (err == nil) != (scenario == "success" || scenario == "revoke success") || fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatalf("confirmation boundary incorrect: %v", err)
			}
		})
	}
}

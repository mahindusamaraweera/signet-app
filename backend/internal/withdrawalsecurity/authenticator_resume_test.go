package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"signet-backend/internal/auth"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatorBeginRetryAndConfirmationPreserveScannedQRAndAttemptLimit(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(59, 0).UTC()
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := encryptEnrollment(key, 42, 7, "user@example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	c, emailCode, err := NewEmailChallenge(key, 42, 7, "user@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = "delivered"
	expires := now.Add(5 * time.Minute)
	created := now.Add(-2 * time.Minute)
	var resumedEncrypted string
	fixture := &sqlFixture{steps: []sqlStep{
		{match: "SELECT password FROM users", values: []driver.Value{password}},
		{match: "SELECT email FROM users", values: []driver.Value{"user@example.com"}},
		{match: "SELECT security_version, withdrawals_frozen", values: []driver.Value{int64(7), false}},
		{match: "FROM withdrawal_security_challenges", values: []driver.Value{c.ID, int64(42), c.Purpose, c.Email, c.Hash, int64(7), c.Status, int64(0), c.ExpiresAt, nil}},
		{match: "SET attempts=attempts+1"}, {match: "SET status='consumed'"}, {match: "SET verified_email=?"}, {match: "INSERT INTO withdrawal_security_audit"},
		{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
		{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, nil, false, int64(8)}},
		{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(7), int64(2), expires, nil, created}},
		{match: "INSERT INTO withdrawal_authenticator_enrollments", check: func(args []driver.NamedValue) error {
			resumedEncrypted = args[1].Value.(string)
			got, err := decryptEnrollment(key, 42, 8, "user@example.com", resumedEncrypted)
			if err != nil || got != secret || args[5].Value != int64(2) || args[6].Value != expires || args[7].Value != created {
				return errors.New("QR, failed attempts or expiry changed on retry")
			}
			return nil
		}},
	}}
	db := sql.OpenDB(fixture)
	defer db.Close()
	s, err := NewService(db, key)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := s.BeginAuthenticator(context.Background(), 42, "test-password", c.ID, emailCode, now)
	if err != nil || setup.Secret != secret || !setup.ExpiresAt.Equal(expires) {
		t.Fatalf("setup retry failed: %v", err)
	}
	fixture.steps = append(fixture.steps,
		sqlStep{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
		sqlStep{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, nil, false, int64(8), nil}},
		sqlStep{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{resumedEncrypted, secretFingerprint(key, ""), "user@example.com", int64(8), int64(2), expires, nil}},
		sqlStep{match: "SET attempts=attempts+1"}, sqlStep{match: "INSERT INTO withdrawal_totp_counters"}, sqlStep{match: "SET last_counter = ?"},
		sqlStep{match: "SET authenticator_fingerprint=?"}, sqlStep{match: "SET confirmed_at=?"}, sqlStep{match: "INSERT INTO withdrawal_security_audit"},
	)
	if err := s.ConfirmAuthenticator(context.Background(), 42, "test-password", "287082", now); err != nil {
		t.Fatalf("first valid QR code rejected: %v", err)
	}
	if len(fixture.steps) != 0 {
		t.Fatal("setup/confirmation flow incomplete")
	}
}

func TestAuthenticatorRetryKeepsScannedSecretOnlyForFreshEmailVersion(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	s := &Service{key: key}
	now := time.Unix(1234567890, 0).UTC()
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	encrypted, err := encryptEnrollment(key, 42, 7, "user@example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	base := pendingAuthenticatorEnrollment{encrypted: encrypted, previousFingerprint: secretFingerprint(key, ""), email: "user@example.com", version: 7, attempts: 2, expires: now.Add(5 * time.Minute), created: now.Add(-2 * time.Minute)}
	for _, scenario := range []string{"resume", "expired", "exhausted", "confirmed", "changed version", "changed email", "changed legacy"} {
		t.Run(scenario, func(t *testing.T) {
			pending := base
			switch scenario {
			case "expired":
				pending.expires = now
			case "exhausted":
				pending.attempts = 5
			case "confirmed":
				pending.confirmed = sql.NullTime{Time: now, Valid: true}
			case "changed version":
				pending.version = 6
			case "changed email":
				pending.email = "changed@example.com"
			case "changed legacy":
				pending.previousFingerprint = secretFingerprint(key, "different")
			}
			resumed, ok, err := s.reusableAuthenticatorSecret(42, 8, "user@example.com", "", now, pending)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "resume" {
				if !ok || resumed != secret || pending.attempts != 2 || !pending.expires.Equal(base.expires) {
					t.Fatal("pending QR or attempts/expiry changed")
				}
			} else if ok || resumed != "" {
				t.Fatal("invalidated setup resumed")
			}
		})
	}
}

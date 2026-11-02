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
	"signet-backend/internal/totp"
)

func TestWithdrawalConfirmationProofAndAuditBoundaries(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "wrong email", "wrong TOTP", "reused TOTP", "expired email", "replaced email", "attempts exhausted", "expired request", "audit failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(59, 0).UTC()
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			if err != nil {
				t.Fatal(err)
			}
			a, proof, err := NewWithdrawalAuthorization(key, terms, now)
			if err != nil {
				t.Fatal(err)
			}
			c, emailCode, err := NewWithdrawalChallenge(key, "user@example.com", a, now)
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := encryptEnrollment(key, 42, 1, c.Email, secret)
			if err != nil {
				t.Fatal(err)
			}
			challengeStatus := "delivered"
			attempts := int64(0)
			expectedErr := error(nil)
			switch scenario {
			case "wrong email":
				expectedErr = ErrWithdrawalEmailIncorrect
			case "wrong TOTP":
				expectedErr = totp.ErrInvalidCode
			case "reused TOTP":
				expectedErr = totp.ErrReusedCode
			case "expired email":
				c.ExpiresAt = now
				expectedErr = ErrWithdrawalEmailExpired
			case "replaced email":
				challengeStatus = "superseded"
				expectedErr = ErrWithdrawalEmailSuperseded
			case "attempts exhausted":
				attempts = 5
				expectedErr = ErrWithdrawalEmailAttempts
			case "expired request":
				a.ExpiresAt = now
				a.ProofHash = a.hash(key, proof)
				expectedErr = ErrWithdrawalExpired
			}
			encoded, _ := json.Marshal(terms)
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{c.Email, password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{c.Email, now, secretFingerprint(key, secret), now, false, int64(2), nil}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), c.Email, int64(1), now}},
				{match: "FROM withdrawal_security_addresses", values: []driver.Value{terms.Destination, now.Add(-24 * time.Hour), now, nil}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "pending", a.ExpiresAt, nil, nil}},
				{match: "FROM withdrawal_security_challenges", values: []driver.Value{c.ID, int64(42), c.Purpose, c.Target, c.Email, c.Hash, int64(2), challengeStatus, attempts, c.ExpiresAt, nil}},
				{match: "SET attempts=attempts+1"},
			}}
			early := scenario == "expired email" || scenario == "replaced email" || scenario == "attempts exhausted" || scenario == "expired request"
			if early {
				fixture.steps = fixture.steps[:len(fixture.steps)-1]
			}
			if scenario == "expired request" {
				fixture.steps = fixture.steps[:len(fixture.steps)-1]
			}
			if scenario == "reused TOTP" {
				zero := int64(0)
				fixture.steps = append(fixture.steps, sqlStep{match: "INSERT INTO withdrawal_totp_counters"}, sqlStep{match: "SET last_counter = ?", affected: &zero})
			}
			code := "287082"
			if scenario == "wrong email" {
				emailCode = "invalid"
			}
			if scenario == "wrong TOTP" {
				code = "invalid"
			}
			if scenario == "success" || scenario == "audit failure" {
				fixture.steps = append(fixture.steps, sqlStep{match: "INSERT INTO withdrawal_totp_counters"}, sqlStep{match: "SET last_counter = ?"}, sqlStep{match: "SET status='consumed'"}, sqlStep{match: "SET status='authorized'"})
				audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
					if args[1].Value != "withdrawal_authorized" || args[2].Value != a.ID {
						return errors.New("wrong authorization audit")
					}
					return nil
				}}
				if scenario == "audit failure" {
					audit.err = errors.New("audit failed")
				}
				fixture.steps = append(fixture.steps, audit)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			err = s.ConfirmWithdrawal(context.Background(), 42, "test-password", a.ID, proof, terms, c.ID, emailCode, code, now)
			commit := scenario != "audit failure" && !early
			if expectedErr != nil && !errors.Is(err, expectedErr) {
				t.Fatalf("wrong diagnostic: got %v want %v", err, expectedErr)
			}
			if (err == nil) != (scenario == "success") || fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatalf("wrong confirmation boundary: %v", err)
			}
		})
	}
}

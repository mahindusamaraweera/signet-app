package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestQueuedAssertionRequiresCurrentSecurityConsumedApprovalAndExactOutbox(t *testing.T) {
	for _, scenario := range []string{"uuid success", "uuid missing", "success", "changed security", "unconsumed", "changed outbox", "already submitted", "changed password", "legacy unbound queue"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			a, _, _ := NewWithdrawalAuthorization(key, terms, now)
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, _ := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			encoded, _ := json.Marshal(terms)
			version := int64(2)
			if scenario == "changed security" {
				version = 3
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", "not-retained-password", nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, version, nil}},
			}}
			if scenario != "changed security" {
				status := "consumed"
				var consumed driver.Value = now
				if scenario == "unconsumed" {
					status = "authorized"
					consumed = nil
				}
				fixture.steps = append(fixture.steps,
					sqlStep{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
					sqlStep{match: "FROM withdrawal_security_addresses", values: []driver.Value{terms.Destination, now.Add(-24 * time.Hour), now, nil}},
					sqlStep{match: "SELECT password FROM users", values: []driver.Value{"not-retained-password"}},
					sqlStep{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, status, a.ExpiresAt, now, consumed}},
				)
				if scenario != "unconsumed" {
					digest, queueStatus := terms.Digest(), "pending"
					if scenario == "changed outbox" {
						digest = strings.Repeat("0", 64)
					}
					if scenario == "already submitted" {
						queueStatus = "submitted"
					}
					var revision driver.Value = withdrawalCredentialDigest(key, 42, "not-retained-password")
					if scenario == "changed password" {
						revision = withdrawalCredentialDigest(key, 42, "old-password-hash")
					}
					if scenario == "legacy unbound queue" {
						revision = nil
					}
					fixture.steps = append(fixture.steps, sqlStep{match: "FROM withdrawal_submission_outbox", values: []driver.Value{int64(42), int64(2), string(encoded), digest, queueStatus, revision}})
				}
			}
			if scenario == "uuid success" || scenario == "uuid missing" {
				value := "01a13b42-28d0-7000-8000-0000000001cd"
				if scenario == "uuid missing" {
					value = ""
				}
				fixture.steps = append(fixture.steps, sqlStep{match: "SELECT uuid FROM users", values: []driver.Value{value}})
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "uuid success" || scenario == "uuid missing" {
				s.RequireUUIDIdentity()
			}
			assertion, signature, err := s.BuildQueuedWithdrawalAssertion(context.Background(), a.ID, terms, []byte(strings.Repeat("z", 32)), now)
			success := scenario == "success" || scenario == "uuid success"
			if (err == nil) != success || fixture.committed != success || len(fixture.steps) != 0 {
				t.Fatalf("wrong signing boundary: %v", err)
			}
			if scenario == "uuid success" && assertion.ExternalUserID != "01a13b42-28d0-7000-8000-0000000001cd" {
				t.Fatal("UUID mode signed a numeric owner")
			}
			if success && (assertion.AuthorizationID != a.ID || len(signature) != 64) {
				t.Fatal("missing signed approval")
			}
			if !success && (signature != "" || assertion.AuthorizationID != "") {
				t.Fatal("failed state returned authority")
			}
		})
	}
}

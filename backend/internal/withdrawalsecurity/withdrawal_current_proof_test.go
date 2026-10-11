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

func TestCurrentProofRequiresSubmittedRemoteLinkAndUnchangedCredentials(t *testing.T) {
	for _, scenario := range []string{"valid", "changed credentials", "wrong queue remote", "wrong approval remote", "not submitted"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			a, _, _ := NewWithdrawalAuthorization(key, terms, now)
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, _ := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			encoded, _ := json.Marshal(terms)
			revision := withdrawalCredentialDigest(key, 42, "current-hash")
			remote := int64(7)
			status := "submitted"
			if scenario == "changed credentials" {
				revision = withdrawalCredentialDigest(key, 42, "old-hash")
			}
			if scenario == "wrong queue remote" {
				remote = 8
			}
			if scenario == "not submitted" {
				status = "pending"
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", "current-hash", nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, int64(2), nil}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
				{match: "FROM withdrawal_security_addresses", values: []driver.Value{terms.Destination, now.Add(-24 * time.Hour), now, nil}},
				{match: "SELECT password FROM users", values: []driver.Value{"current-hash"}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "consumed", a.ExpiresAt, now, now}},
				{match: "FROM withdrawal_submission_outbox", values: []driver.Value{int64(42), int64(2), string(encoded), terms.Digest(), status, revision, remote}},
			}}
			if scenario == "valid" || scenario == "wrong approval remote" {
				approvedRemote := int64(7)
				if scenario == "wrong approval remote" {
					approvedRemote = 8
				}
				fixture.steps = append(fixture.steps, sqlStep{match: "SELECT remote_withdrawal_id", values: []driver.Value{approvedRemote}})
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, _ := NewService(db, key)
			proof, err := s.BuildWithdrawalCurrentProof(context.Background(), a.ID, terms, 7, strings.Repeat("e", 64), []byte(strings.Repeat("z", 32)), now)
			success := scenario == "valid"
			if (err == nil) != success || fixture.committed != success || len(fixture.steps) != 0 {
				t.Fatalf("wrong proof boundary: %v", err)
			}
			if !success && proof.Signature != "" {
				t.Fatal("invalid current state returned proof")
			}
		})
	}
}

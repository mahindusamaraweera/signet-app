package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/withdrawalsecurity"
)

func TestWithdrawalEmailVerificationTransactionBoundaries(t *testing.T) {
	for _, scenario := range []string{"success", "wrong code", "changed email", "changed security version", "exhausted", "profile write failure", "audit write failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0)
			challenge, code, err := withdrawalsecurity.NewEmailChallenge(key, 42, 1, "user@example.com", now)
			if err != nil {
				t.Fatal(err)
			}
			challenge.Status = "delivered"
			currentEmail := challenge.Email
			version := int64(1)
			if scenario == "changed email" {
				currentEmail = "new@example.com"
			}
			if scenario == "changed security version" {
				version = 2
			}
			if scenario == "exhausted" {
				challenge.Attempts = 5
			}
			if scenario == "wrong code" {
				if code == "000000" {
					code = "111111"
				} else {
					code = "000000"
				}
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT email FROM users WHERE id=? FOR UPDATE", columns: []string{"email"}, values: []driver.Value{currentEmail}, args: []driver.Value{int64(42)}},
				{contains: "SELECT security_version, withdrawals_frozen", columns: []string{"security_version", "withdrawals_frozen"}, values: []driver.Value{version, false}},
				{contains: "FROM withdrawal_security_challenges WHERE id=? AND user_id=? FOR UPDATE", columns: []string{"id", "user_id", "purpose", "email", "code_hash", "security_version", "status", "attempts", "expires_at", "consumed_at"},
					values: []driver.Value{challenge.ID, int64(42), challenge.Purpose, challenge.Email, challenge.Hash, int64(1), challenge.Status, int64(challenge.Attempts), challenge.ExpiresAt, nil},
					args:   []driver.Value{challenge.ID, int64(42)}},
			}}
			if scenario != "exhausted" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SET attempts=attempts+1", args: []driver.Value{challenge.ID, int64(42)}})
			}
			if scenario == "success" || scenario == "profile write failure" || scenario == "audit write failure" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SET status='consumed',consumed_at=?", args: []driver.Value{now.UTC(), challenge.ID, int64(42)}})
				step := commissionSQLStep{contains: "SET verified_email=?,email_verified_at=?,security_version=security_version+1", args: []driver.Value{currentEmail, now.UTC(), int64(42), int64(1)}}
				if scenario == "profile write failure" {
					step.err = errors.New("profile write failed")
				}
				fixture.steps = append(fixture.steps, step)
				if scenario != "profile write failure" {
					audit := commissionSQLStep{contains: "INSERT INTO withdrawal_security_audit", args: []driver.Value{int64(42), "email_verified", challenge.ID, int64(2), now.UTC()}}
					if scenario == "audit write failure" {
						audit.err = errors.New("audit write failed")
					}
					fixture.steps = append(fixture.steps, audit)
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			service, err := withdrawalsecurity.NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			err = service.VerifyEmail(context.Background(), 42, challenge.ID, code, now)
			if scenario == "success" {
				if err != nil || !fixture.committed || fixture.rolledBack {
					t.Fatalf("verification failed: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("invalid verification accepted")
				}
				commitAttempt := scenario == "wrong code" || scenario == "changed email" || scenario == "changed security version"
				if fixture.committed != commitAttempt || fixture.rolledBack == commitAttempt {
					t.Fatalf("wrong transaction boundary: commit=%v rollback=%v", fixture.committed, fixture.rolledBack)
				}
			}
			if len(fixture.steps) != 0 {
				t.Fatalf("%d SQL steps not executed", len(fixture.steps))
			}
		})
	}
}

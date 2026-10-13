package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"signet-backend/internal/auth"
)

func TestPasswordResetRejectsExpiredOrFutureProofWithoutMutation(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	for _, created := range []time.Time{now.Add(-time.Hour), now.Add(time.Second)} {
		fixture := &commissionSQLFixture{steps: []commissionSQLStep{
			{contains: "SELECT id FROM users", columns: []string{"id"}, values: []driver.Value{int64(42)}},
			{contains: "SELECT created_at FROM password_reset", columns: []string{"created_at"}, values: []driver.Value{created}, args: []driver.Value{"user@example.com", "reset-token"}},
		}}
		db := sql.OpenDB(fixture)
		err := completePasswordReset(context.Background(), db, "user@example.com", "reset-token", "test-password", now)
		db.Close()
		if !errors.Is(err, errResetProof) || fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
			t.Fatalf("invalid reset accepted: %v", err)
		}
	}
}

func TestPasswordResetSuccessAndAtomicFailureBoundaries(t *testing.T) {
	for _, failure := range []string{"", "password", "profile", "challenges", "authorizations", "token deletion", "audit"} {
		t.Run("failure="+failure, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			hold := now.Add(72 * time.Hour)
			steps := []commissionSQLStep{
				{contains: "SELECT id FROM users", columns: []string{"id"}, values: []driver.Value{int64(42)}, args: []driver.Value{"user@example.com"}},
				{contains: "SELECT created_at FROM password_reset", columns: []string{"created_at"}, values: []driver.Value{now.Add(-time.Minute)}, args: []driver.Value{"user@example.com", "reset-token"}},
				{contains: "INSERT INTO withdrawal_security_profiles"},
				{contains: "SELECT security_version,hold_until", columns: []string{"security_version", "hold_until"}, values: []driver.Value{int64(2), hold}},
			}
			writes := []struct {
				name string
				step commissionSQLStep
			}{
				{"password", commissionSQLStep{contains: "UPDATE users SET password", checkArgs: func(args []driver.NamedValue) error {
					if !auth.CheckPassword(args[0].Value.(string), "test-password") || args[1].Value != now || args[2].Value != int64(42) {
						return errors.New("wrong password update")
					}
					return nil
				}}},
				{"profile", commissionSQLStep{contains: "SET security_version=security_version+1,hold_until", args: []driver.Value{hold, int64(42), int64(2)}}},
				{"challenges", commissionSQLStep{contains: "UPDATE withdrawal_security_challenges", args: []driver.Value{int64(42)}}},
				{"authorizations", commissionSQLStep{contains: "UPDATE withdrawal_authorizations", args: []driver.Value{int64(42)}}},
				{"token deletion", commissionSQLStep{contains: "DELETE FROM password_reset", args: []driver.Value{"user@example.com"}}},
				{"audit", commissionSQLStep{contains: "INSERT INTO withdrawal_security_audit", checkArgs: func(args []driver.NamedValue) error {
					if args[0].Value != int64(42) || len(args[1].Value.(string)) != 64 || args[1].Value == "reset-token" || args[2].Value != int64(3) || args[3].Value != now {
						return errors.New("wrong reset audit")
					}
					return nil
				}}},
			}
			for _, write := range writes {
				step := write.step
				if write.name == failure {
					step.err = errors.New("simulated " + failure + " failure")
				}
				steps = append(steps, step)
				if write.name == failure {
					break
				}
			}
			fixture := &commissionSQLFixture{steps: steps}
			db := sql.OpenDB(fixture)
			defer db.Close()
			err := completePasswordReset(context.Background(), db, "user@example.com", "reset-token", "test-password", now)
			commit := failure == ""
			if (err == nil) != commit || fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatalf("wrong reset boundary: %v", err)
			}
		})
	}
}

package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/registrationverification"
)

type registrationTestMailer struct{}

func (registrationTestMailer) Send(context.Context, string, string, string) error { return nil }

func TestVerifiedRegistrationTransactionBoundaries(t *testing.T) {
	for _, scenario := range []string{"success", "invalid token", "expired", "enrollment failure", "wallet failure", "consumption failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0)
			pending, token, err := registrationverification.New(key, "user@example.com", now)
			if err != nil {
				t.Fatal(err)
			}
			pending.Status = "delivered"
			submitted := token
			if scenario == "invalid token" {
				submitted = strings.Repeat("0", 64)
			}
			if scenario == "expired" {
				now = pending.ExpiresAt
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "registration_completion_locks", columns: []string{"id"}, values: []driver.Value{int64(1)}},
				{contains: "SELECT email FROM pending_email_registrations", columns: []string{"email"}, values: []driver.Value{pending.Email}},
				{contains: "registration_email_guards", columns: []string{"email"}, values: []driver.Value{pending.Email}},
				{contains: "SELECT id,email,verification_hash", columns: []string{"id", "email", "verification_hash", "status", "expires_at", "verified_at", "registration_data", "password_hash"},
					values: []driver.Value{pending.ID, pending.Email, pending.VerificationHash, pending.Status, pending.ExpiresAt, nil,
						`{"name":"Investor","whatsapp_number":"+12345","country_id":1,"referral_code":"ABC123"}`, "stored-password-hash"}},
			}}
			if scenario != "invalid token" && scenario != "expired" {
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "SELECT COUNT(*) FROM users WHERE email", columns: []string{"count"}, values: []driver.Value{int64(0)}},
					commissionSQLStep{contains: "SELECT user_id FROM referral_codes", columns: []string{"user_id"}, values: []driver.Value{int64(7)}},
					commissionSQLStep{contains: "SELECT COUNT(*) FROM countries", columns: []string{"count"}, values: []driver.Value{int64(1)}},
					commissionSQLStep{contains: "INSERT INTO users", insertID: 42},
					commissionSQLStep{contains: "INSERT INTO registration_expiry_enrollments", args: []driver.Value{int64(42), now.UTC()}},
				)
				if scenario == "enrollment failure" {
					fixture.steps[len(fixture.steps)-1].err = errors.New("expiry enrollment failed")
				} else if scenario == "wallet failure" {
					fixture.steps = append(fixture.steps, commissionSQLStep{contains: "INSERT INTO wallets", err: errors.New("wallet write failed")})
				} else {
					fixture.steps = append(fixture.steps,
						commissionSQLStep{contains: "INSERT INTO wallets"}, commissionSQLStep{contains: "INSERT INTO referral_codes"},
						commissionSQLStep{contains: "INSERT INTO user_secret_keys"}, commissionSQLStep{contains: "INSERT INTO withdrawal_security_profiles"},
					)
					last := commissionSQLStep{contains: "UPDATE pending_email_registrations SET status='verified'"}
					if scenario == "consumption failure" {
						last.err = errors.New("consumption write failed")
					}
					fixture.steps = append(fixture.steps, last)
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			service, err := registrationverification.NewService(db, key, "https://app.example.com", registrationTestMailer{})
			if err != nil {
				t.Fatal(err)
			}
			id, err := service.Complete(context.Background(), pending.ID, submitted, now)
			if scenario == "success" {
				if err != nil || id != 42 || !fixture.committed || fixture.rolledBack {
					t.Fatalf("success: id=%d error=%v committed=%v rollback=%v", id, err, fixture.committed, fixture.rolledBack)
				}
			} else if err == nil || id != 0 || fixture.committed || !fixture.rolledBack {
				t.Fatalf("failure boundary: id=%d error=%v committed=%v rollback=%v", id, err, fixture.committed, fixture.rolledBack)
			}
			if len(fixture.steps) != 0 {
				t.Fatalf("%d expected SQL steps not executed", len(fixture.steps))
			}
		})
	}
}

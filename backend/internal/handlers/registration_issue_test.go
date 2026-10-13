package handlers

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
	"signet-backend/internal/registrationverification"
)

type recordingRegistrationMailer struct {
	calls int
	fail  bool
	body  string
}

func (m *recordingRegistrationMailer) Send(_ context.Context, _, _, body string) error {
	m.calls++
	m.body = body
	if m.fail {
		return errors.New("provider unavailable")
	}
	return nil
}

func TestPendingRegistrationIssuanceNeverCreatesAccount(t *testing.T) {
	for _, scenario := range []string{"accepted", "delivery failure", "resend cooldown"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0)
			recent := int64(0)
			if scenario == "resend cooldown" {
				recent = 1
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "INSERT INTO registration_email_guards"},
				{contains: "SELECT email FROM registration_email_guards", columns: []string{"email"}, values: []driver.Value{"user@example.com"}},
				{contains: "SELECT COUNT(*) FROM users WHERE email", columns: []string{"count"}, values: []driver.Value{int64(0)}},
				{contains: "SELECT COUNT(*) FROM pending_email_registrations", columns: []string{"count"}, values: []driver.Value{recent}},
				{contains: "SELECT COUNT(*) FROM pending_email_registrations", columns: []string{"count"}, values: []driver.Value{int64(0)}},
			}}
			var storedID string
			if scenario != "resend cooldown" {
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "UPDATE pending_email_registrations SET status='superseded'"},
					commissionSQLStep{contains: "INSERT INTO pending_email_registrations", checkArgs: func(args []driver.NamedValue) error {
						storedID = args[0].Value.(string)
						payload := args[2].Value.(string)
						var data map[string]interface{}
						if err := json.Unmarshal([]byte(payload), &data); err != nil {
							return err
						}
						if _, exists := data["password"]; exists || strings.Contains(payload, "test-password") {
							return errors.New("plaintext password persisted")
						}
						if !auth.CheckPassword(args[3].Value.(string), "test-password") {
							return errors.New("password was not hashed")
						}
						if len(args[4].Value.(string)) != 64 {
							return errors.New("verification hash missing")
						}
						return nil
					}},
					commissionSQLStep{contains: "UPDATE pending_email_registrations SET status=?", checkArgs: func(args []driver.NamedValue) error {
						want := "delivered"
						if scenario == "delivery failure" {
							want = "delivery_failed"
						}
						if args[0].Value != want || args[1].Value != storedID {
							return errors.New("incorrect delivery outcome")
						}
						return nil
					}},
				)
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			mailer := &recordingRegistrationMailer{fail: scenario == "delivery failure"}
			service, err := registrationverification.NewService(db, []byte(strings.Repeat("k", 32)), "https://app.example.com", mailer)
			if err != nil {
				t.Fatal(err)
			}
			id, err := service.Issue(context.Background(), "user@example.com", "test-password", registrationverification.Data{Name: "Investor", Whatsapp: "+12345", CountryID: 1, ReferralCode: "ABC123"}, now)
			if scenario == "accepted" {
				if err != nil || id != storedID || !fixture.committed || mailer.calls != 1 {
					t.Fatalf("issuance failed: %v", err)
				}
				if !strings.Contains(mailer.body, "/register/verify#") {
					t.Fatal("verification secret not in fragment")
				}
			} else if err == nil || id != "" {
				t.Fatal("failed issuance returned usable identity")
			}
			if scenario == "resend cooldown" && (mailer.calls != 0 || fixture.committed || !fixture.rolledBack) {
				t.Fatal("cooldown did not stop issuance")
			}
			if scenario == "delivery failure" && (!fixture.committed || mailer.calls != 1) {
				t.Fatal("failure not recorded after pending commit")
			}
			if len(fixture.steps) != 0 {
				t.Fatalf("%d SQL steps not executed", len(fixture.steps))
			}
		})
	}
}

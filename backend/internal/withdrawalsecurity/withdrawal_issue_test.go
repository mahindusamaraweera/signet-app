package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/auth"
)

type withdrawalTestMailer struct {
	fixture *sqlFixture
	calls   int
	fail    bool
	terms   WithdrawalTerms
	code    string
}

func (m *withdrawalTestMailer) Send(_ context.Context, to, subject, body string) error {
	m.calls++
	if !m.fixture.committed || to != "user@example.com" || subject != "Confirm SIGNET withdrawal" {
		return errors.New("mail before commit or wrong recipient")
	}
	for _, term := range []string{m.terms.Destination, m.terms.Gross, m.terms.Fee, m.terms.Net} {
		if !strings.Contains(body, term) {
			return errors.New("missing immutable terms")
		}
	}
	match := regexp.MustCompile(`Verification code: ([0-9]{6})`).FindStringSubmatch(body)
	if len(match) != 2 {
		return errors.New("missing code")
	}
	m.code = match[1]
	if m.fail {
		return errors.New("SMTP rejected")
	}
	return nil
}

func TestWithdrawalEmailIssuanceDeliveryAndAuditBoundaries(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "cooldown", "quota", "audit failure", "mail failure", "record failure"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
			if err != nil {
				t.Fatal(err)
			}
			a, proof, err := NewWithdrawalAuthorization(key, terms, now)
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(terms)
			recent, hourly := int64(0), int64(0)
			if scenario == "cooldown" {
				recent = 1
			}
			if scenario == "quota" {
				hourly = 5
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, int64(2), nil}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
				{match: "FROM withdrawal_security_addresses", values: []driver.Value{terms.Destination, now.Add(-24 * time.Hour), now, nil}},
				{match: "FROM withdrawal_authorizations", values: []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "pending", a.ExpiresAt, nil, nil}},
				{match: "SELECT COUNT(*)", values: []driver.Value{recent}}, {match: "SELECT COUNT(*)", values: []driver.Value{hourly}},
			}}
			var stored Challenge
			if recent == 0 && hourly < 5 {
				fixture.steps = append(fixture.steps, sqlStep{match: "SET status='superseded'"}, sqlStep{match: "INSERT INTO withdrawal_security_challenges", check: func(args []driver.NamedValue) error {
					stored = Challenge{ID: args[0].Value.(string), UserID: 42, Purpose: args[2].Value.(string), Target: args[3].Value.(string), Email: args[4].Value.(string), Hash: args[5].Value.(string), SecurityVersion: 2, Status: "delivered", ExpiresAt: args[7].Value.(time.Time)}
					return nil
				}})
				audit := sqlStep{match: "INSERT INTO withdrawal_security_audit", check: func(args []driver.NamedValue) error {
					if args[1].Value != "withdrawal_email_requested" || args[2].Value != stored.ID {
						return errors.New("wrong audit reference")
					}
					return nil
				}}
				if scenario == "audit failure" {
					audit.err = errors.New("audit failed")
				}
				fixture.steps = append(fixture.steps, audit)
				if scenario != "audit failure" {
					record := sqlStep{match: "SET status=?", check: func(args []driver.NamedValue) error {
						status := "delivered"
						if scenario == "mail failure" {
							status = "delivery_failed"
						}
						if args[0].Value != status || args[1].Value != stored.ID {
							return errors.New("wrong delivery result")
						}
						return nil
					}}
					if scenario == "record failure" {
						record.err = errors.New("record failed")
					}
					fixture.steps = append(fixture.steps, record)
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			mailer := &withdrawalTestMailer{fixture: fixture, fail: scenario == "mail failure", terms: terms}
			id, err := s.IssueWithdrawalEmail(context.Background(), 42, "test-password", a.ID, proof, terms, now, mailer)
			if scenario == "success" {
				if err != nil || id != stored.ID || !stored.AcceptsWithdrawal(key, stored.Email, mailer.code, a, now) {
					t.Fatalf("invalid successful issuance: %v", err)
				}
			} else if err == nil || id != "" {
				t.Fatal("failed issuance returned challenge")
			}
			commit := scenario == "success" || scenario == "mail failure" || scenario == "record failure"
			if fixture.committed != commit || fixture.rolledBack == commit || (mailer.calls == 1) != commit || len(fixture.steps) != 0 {
				t.Fatal("wrong email/transaction boundary")
			}
		})
	}
}

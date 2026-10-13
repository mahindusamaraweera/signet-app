package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/auth"
)

type addressTestMailer struct {
	fixture     *sqlFixture
	calls       int
	code        string
	fail        bool
	destination string
}

func (m *addressTestMailer) Send(_ context.Context, to, subject, body string) error {
	m.calls++
	if !m.fixture.committed || to != "user@example.com" || !strings.Contains(body, m.destination) || !strings.Contains(subject, "withdrawal address") {
		return errors.New("mail sent before commit or to wrong recipient")
	}
	match := regexp.MustCompile(`Verification code: ([0-9]{6})`).FindStringSubmatch(body)
	if len(match) != 2 {
		return errors.New("missing verification code")
	}
	m.code = match[1]
	if m.fail {
		return errors.New("SMTP rejected")
	}
	return nil
}

func TestAddressIssuanceDeliveryAndCooldownBoundaries(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "delivery failure", "record failure", "cooldown", "hourly limit", "insert failure", "revoke success"} {
		t.Run(scenario, func(t *testing.T) {
			key := []byte(strings.Repeat("k", 32))
			now := time.Unix(1800000000, 0).UTC()
			destination := "0x" + strings.Repeat("a", 40)
			secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
			encrypted, err := encryptEnrollment(key, 42, 1, "user@example.com", secret)
			if err != nil {
				t.Fatal(err)
			}
			recent, hourly := int64(0), int64(0)
			if scenario == "cooldown" {
				recent = 1
			}
			if scenario == "hourly limit" {
				hourly = 5
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{"user@example.com", now, secretFingerprint(key, secret), now, false, int64(2)}},
				{match: "FROM withdrawal_authenticator_enrollments", values: []driver.Value{encrypted, secretFingerprint(key, ""), "user@example.com", int64(1), now}},
				{match: "SELECT COUNT(*)", values: []driver.Value{recent}}, {match: "SELECT COUNT(*)", values: []driver.Value{hourly}},
			}}
			var stored Challenge
			purpose := AddressEnrollmentPurpose
			addressID := strings.Repeat("b", 64)
			if scenario == "revoke success" {
				purpose = AddressRevocationPurpose
				before := append([]sqlStep(nil), fixture.steps[:3]...)
				before = append(before, sqlStep{match: "SELECT address,revoked_at", values: []driver.Value{destination, nil}, check: func(args []driver.NamedValue) error {
					if args[0].Value != addressID || args[1].Value != int64(42) {
						return errors.New("wrong revocation owner")
					}
					return nil
				}})
				fixture.steps = append(before, fixture.steps[3:]...)
			}
			mailer := &addressTestMailer{fixture: fixture, fail: scenario == "delivery failure", destination: destination}
			if recent == 0 && hourly < 5 {
				fixture.steps = append(fixture.steps, sqlStep{match: "SET status='superseded'"})
				insert := sqlStep{match: "INSERT INTO withdrawal_security_challenges", check: func(args []driver.NamedValue) error {
					stored = Challenge{ID: args[0].Value.(string), UserID: 42, Purpose: args[2].Value.(string), Target: args[3].Value.(string), Email: args[4].Value.(string), Hash: args[5].Value.(string), SecurityVersion: 2, Status: "delivered", ExpiresAt: args[7].Value.(time.Time)}
					if stored.Purpose != purpose || stored.ExpiresAt != now.Add(10*time.Minute) {
						return errors.New("invalid stored proof")
					}
					return nil
				}}
				if scenario == "insert failure" {
					insert.err = errors.New("insert failed")
				}
				fixture.steps = append(fixture.steps, insert)
				if scenario != "insert failure" {
					record := sqlStep{match: "SET status=?", check: func(args []driver.NamedValue) error {
						status := "delivered"
						if scenario == "delivery failure" {
							status = "delivery_failed"
						}
						if args[0].Value != status || args[1].Value != stored.ID || args[2].Value != int64(42) {
							return errors.New("wrong delivery state")
						}
						return nil
					}}
					if scenario == "record failure" {
						record.err = errors.New("delivery record failed")
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
			var id string
			if scenario == "revoke success" {
				id, err = s.IssueAddressRevocation(context.Background(), 42, "test-password", addressID, now, mailer)
			} else {
				id, err = s.IssueAddress(context.Background(), 42, "test-password", destination, now, mailer)
			}
			if scenario == "revoke success" {
				if err != nil || id != stored.ID || !stored.AcceptsAddressRevocation(key, 42, 2, stored.Email, addressID, mailer.code, now) {
					t.Fatalf("invalid revocation issuance: %v", err)
				}
			} else if scenario == "success" {
				if err != nil || id != stored.ID || !stored.AcceptsAddress(key, 42, 2, stored.Email, destination, mailer.code, now) {
					t.Fatalf("invalid successful issuance: %v", err)
				}
			} else if err == nil || id != "" {
				t.Fatal("failed issuance returned usable identity")
			}
			commit := scenario == "success" || scenario == "revoke success" || scenario == "delivery failure" || scenario == "record failure"
			if fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 || (mailer.calls == 1) != commit {
				t.Fatal("wrong issuance transaction/delivery boundary")
			}
		})
	}
}

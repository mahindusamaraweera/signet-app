package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"
)

type resetTestMailer struct {
	fixture *commissionSQLFixture
	calls   int
	fail    bool
	token   string
}

func (m *resetTestMailer) Send(_ context.Context, to, subject, body string) error {
	m.calls++
	if !m.fixture.committed || to != "user@example.com" || subject != "Password Reset" {
		return errors.New("mail before commit or wrong recipient")
	}
	match := regexp.MustCompile(`https://app.example.com/password/reset/verify#token=(rt1_[a-f0-9]{64})`).FindStringSubmatch(body)
	if len(match) != 2 {
		return errors.New("invalid reset link")
	}
	m.token = match[1]
	if m.fail {
		return errors.New("SMTP failed")
	}
	return nil
}

func TestResetIssuanceDeliveryAndDatabaseBoundaries(t *testing.T) {
	for _, scenario := range []string{"success", "unknown email", "cooldown", "hourly quota", "insert failure", "mail failure", "record failure"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Unix(1800000000, 0).UTC()
			owner := commissionSQLStep{contains: "SELECT id FROM users", columns: []string{"id"}, values: []driver.Value{int64(42)}, args: []driver.Value{"user@example.com"}}
			if scenario == "unknown email" {
				owner.err = sql.ErrNoRows
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{owner}}
			var stored string
			if scenario != "unknown email" {
				recent, hourly := int64(0), int64(0)
				if scenario == "cooldown" {
					recent = 1
				}
				if scenario == "hourly quota" {
					hourly = 5
				}
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SELECT COUNT(*)", columns: []string{"count"}, values: []driver.Value{recent}}, commissionSQLStep{contains: "SELECT COUNT(*)", columns: []string{"count"}, values: []driver.Value{hourly}})
				if recent == 0 && hourly < 5 {
					fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SET delivery_status='superseded'"})
					insert := commissionSQLStep{contains: "INSERT INTO password_reset", checkArgs: func(args []driver.NamedValue) error {
						stored = args[1].Value.(string)
						if args[0].Value != "user@example.com" || len(stored) != 71 || args[2].Value != now {
							return errors.New("invalid pending proof")
						}
						return nil
					}}
					if scenario == "insert failure" {
						insert.err = errors.New("insert failed")
					}
					fixture.steps = append(fixture.steps, insert)
					if scenario != "insert failure" {
						record := commissionSQLStep{contains: "SET delivery_status=?", checkArgs: func(args []driver.NamedValue) error {
							expected := "delivered"
							if scenario == "mail failure" {
								expected = "delivery_failed"
							}
							if args[0].Value != expected || args[1].Value != "user@example.com" || args[2].Value != stored {
								return errors.New("wrong delivery state")
							}
							return nil
						}}
						if scenario == "record failure" {
							record.err = errors.New("record failed")
						}
						fixture.steps = append(fixture.steps, record)
					}
				}
			}
			mailer := &resetTestMailer{fixture: fixture, fail: scenario == "mail failure"}
			db := sql.OpenDB(fixture)
			defer db.Close()
			err := issuePasswordReset(context.Background(), db, "user@example.com", "https://app.example.com", now, mailer)
			if (err == nil) != (scenario == "success" || scenario == "unknown email") {
				t.Fatalf("wrong issuance outcome: %v", err)
			}
			commit := scenario == "success" || scenario == "mail failure" || scenario == "record failure"
			if fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 || (mailer.calls == 1) != commit {
				t.Fatal("wrong delivery/transaction boundary")
			}
			if mailer.calls == 1 {
				lookup, err := passwordResetLookup(mailer.token)
				if err != nil || lookup != stored {
					t.Fatal("email secret differs from stored hash")
				}
			}
		})
	}
}

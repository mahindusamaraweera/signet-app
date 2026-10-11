package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/auth"
)

func TestWithdrawalCurrentStateRejectsSecurityChanges(t *testing.T) {
	password, err := auth.HashPassword("test-password")
	if err != nil {
		t.Fatal(err)
	}
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"changed version", "frozen", "new hold", "changed email"} {
		t.Run(scenario, func(t *testing.T) {
			version := int64(2)
			frozen := false
			var hold driver.Value
			verified := "user@example.com"
			if scenario == "changed version" {
				version++
			}
			if scenario == "frozen" {
				frozen = true
			}
			if scenario == "new hold" {
				hold = now.Add(time.Hour)
			}
			if scenario == "changed email" {
				verified = "old@example.com"
			}
			fixture := &sqlFixture{steps: []sqlStep{
				{match: "SELECT email,password,google_authenticator_secret", values: []driver.Value{"user@example.com", password, nil}},
				{match: "FROM withdrawal_security_profiles", values: []driver.Value{verified, now, nil, now, frozen, version, hold}},
			}}
			db := sql.OpenDB(fixture)
			defer db.Close()
			s, err := NewService(db, key)
			if err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			email, secret, err := s.lockWithdrawalState(context.Background(), tx, terms, "test-password", now)
			if err == nil || email != "" || secret != "" || len(fixture.steps) != 0 {
				t.Fatal("changed account returned authorization credentials")
			}
		})
	}
}

package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWithdrawalConsumptionConditionalUpdateAndAuditAtomicity(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "already consumed", "SQL failure", "audit failure", "wrong proof"} {
		t.Run(scenario, func(t *testing.T) {
			a, proof, err := NewWithdrawalAuthorization(key, terms, now)
			if err != nil {
				t.Fatal(err)
			}
			a.Status = "authorized"
			a.AuthorizedAt = &now
			fixture := &sqlFixture{}
			if scenario == "wrong proof" {
				proof = strings.Repeat("c", 64)
			} else {
				update := sqlStep{match: "SET status='consumed'", check: func(args []driver.NamedValue) error {
					if args[1].Value != a.ID || args[2].Value != int64(42) || args[3].Value != int64(2) || args[4].Value != terms.Digest() || args[5].Value != a.ProofHash {
						return errors.New("wrong consumption binding")
					}
					return nil
				}}
				if scenario == "already consumed" {
					zero := int64(0)
					update.affected = &zero
				}
				if scenario == "SQL failure" {
					update.err = errors.New("update failed")
				}
				fixture.steps = append(fixture.steps, update)
				if scenario == "success" || scenario == "audit failure" {
					audit := sqlStep{match: "INSERT INTO withdrawal_security_audit"}
					if scenario == "audit failure" {
						audit.err = errors.New("audit failed")
					}
					fixture.steps = append(fixture.steps, audit)
				}
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			err = consumeWithdrawalAuthorization(context.Background(), tx, a, key, proof, terms, now)
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
			commit := scenario == "success"
			if (err == nil) != commit || fixture.committed != commit || fixture.rolledBack == commit || len(fixture.steps) != 0 {
				t.Fatalf("wrong consumption boundary: %v", err)
			}
		})
	}
}

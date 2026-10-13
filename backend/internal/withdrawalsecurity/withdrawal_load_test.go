package withdrawalsecurity

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLockedWithdrawalAuthorizationRejectsInconsistentStoredEvidence(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := NewWithdrawalAuthorization([]byte(strings.Repeat("k", 32)), terms, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"valid", "owner", "version", "address", "digest", "amount", "malformed", "pending timestamps", "authorized timestamp missing", "consumed timestamp missing"} {
		t.Run(scenario, func(t *testing.T) {
			encoded, _ := json.Marshal(terms)
			values := []driver.Value{a.ID, int64(42), int64(2), terms.AddressID, string(encoded), terms.Digest(), a.ProofHash, "pending", a.ExpiresAt, nil, nil}
			switch scenario {
			case "owner":
				values[1] = int64(43)
			case "version":
				values[2] = int64(3)
			case "address":
				values[3] = strings.Repeat("c", 64)
			case "digest":
				values[5] = strings.Repeat("c", 64)
			case "amount":
				copy := terms
				copy.Fee = "4"
				data, _ := json.Marshal(copy)
				values[4] = string(data)
			case "malformed":
				values[4] = "{}"
			case "pending timestamps":
				values[9] = now
			case "authorized timestamp missing":
				values[7] = "authorized"
			case "consumed timestamp missing":
				values[7] = "consumed"
				values[9] = now
			}
			fixture := &sqlFixture{steps: []sqlStep{{match: "WHERE id=? AND user_id=? FOR UPDATE", values: values, check: func(args []driver.NamedValue) error {
				if args[0].Value != a.ID || args[1].Value != int64(42) {
					return errors.New("wrong scope")
				}
				return nil
			}}}}
			db := sql.OpenDB(fixture)
			defer db.Close()
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			loaded, err := loadWithdrawalAuthorization(context.Background(), tx, 42, a.ID)
			if (err == nil) != (scenario == "valid") || (scenario == "valid" && loaded.Terms != terms) || len(fixture.steps) != 0 {
				t.Fatalf("invalid stored evidence result: %v", err)
			}
		})
	}
}

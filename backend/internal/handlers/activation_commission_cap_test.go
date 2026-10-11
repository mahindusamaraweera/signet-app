package handlers

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestAllIntegratedActivationBonusKindsDivertAtCap(t *testing.T) {
	for _, kind := range []string{"direct_team", "spillover", "gratitude", "leadership", "executive"} {
		t.Run(kind, func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT id FROM users WHERE id=? FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(77)}},
				{contains: "SELECT balance FROM wallets", columns: []string{"balance"}, values: []driver.Value{float64(400)}},
				{contains: "SUM(p.price*4)", columns: []string{"cap"}, values: []driver.Value{float64(400)}},
				{contains: "SELECT id FROM users WHERE id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(1)}},
				{contains: "SELECT id FROM wallets WHERE user_id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(11)}},
				{contains: "UPDATE wallets SET balance=balance+?", args: []driver.Value{float64(5), int64(11)}},
				{contains: "INSERT INTO earn_logs", insertID: 18},
				{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(1), int64(18), int64(1), "5", "Income cap commission diversion", kind}},
				{contains: "INSERT INTO income_cap_diversions", args: []driver.Value{int64(77), int64(1), "5", kind}},
			}}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			tx, err := db.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if err := creditActivationCommissionTx(tx, 77, 5, "", kind, activationCommissionContext{IntentID: 1}); err != nil {
				t.Fatal(err)
			}
			if len(fixture.steps) != 0 {
				t.Fatal("company diversion evidence missing")
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

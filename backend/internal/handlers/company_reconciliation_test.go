package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestUnreconciledCompanyAccountingCannotRemainInPayoutQueue(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(5), int64(18), int64(1), "50", "Leadership Bonus", "leadership"}},
		{contains: "UPDATE blockchain_commission_outbox SET status='requires_reconciliation'", args: []driver.Value{int64(18)}, err: errors.New("reconciliation marker failed")},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	err = purchaseCommissionRecorder(5, "leadership")(tx, 18, 1, "50", "Leadership Bonus")
	if err == nil || err.Error() != "reconciliation marker failed" {
		t.Fatalf("unsafe company record accepted: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("unsafe company instruction committed")
	}
}

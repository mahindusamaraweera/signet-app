package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/jmoiron/sqlx"
	"signet-backend/internal/wallet"
	"testing"
)

func TestPendingCommissionGuardUsesBeneficiaryLockAndBlocksLegacyCredit(t *testing.T) {
	wallet.ConfigurePendingCommissionGuard(true)
	defer wallet.ConfigurePendingCommissionGuard(false)
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT id FROM users WHERE id=? FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(77)}},
		{contains: "SELECT COUNT(*) FROM blockchain_topup_commission_plans", columns: []string{"count"}, values: []driver.Value{int64(1)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if !errors.Is(wallet.EnsureNoPendingCommissions(tx, 77), wallet.ErrPendingCommissions) {
		t.Fatal("pending reservation was ignored")
	}
	if len(fixture.steps) != 0 {
		t.Fatal("guard did not lock beneficiary")
	}
}

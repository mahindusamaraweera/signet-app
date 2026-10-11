package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/jmoiron/sqlx"
	"signet-backend/internal/blockchain"
	"testing"
)

func TestTopupCannotExtendCapWithoutCommittedPlan(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "SELECT commission_plan_created", columns: []string{"commission_plan_created"}, values: []driver.Value{false}}}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	if completeBlockchainTopup(tx, blockchain.PurchaseIntent{ID: 7, UserID: 42}) == nil {
		t.Fatal("missing plan extended cap")
	}
	tx.Rollback()
	if fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("unsafe transaction outcome")
	}
}

func TestTopupAccountingFailureCannotActivatePackage(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT commission_plan_created", columns: []string{"commission_plan_created"}, values: []driver.Value{true}},
		{contains: "FROM blockchain_topup_commission_plans", columns: []string{"id", "original_user_id", "beneficiary_user_id", "commission_kind", "commission_usd"}, values: []driver.Value{int64(1), int64(77), int64(1), "direct_team", "50"}},
		{contains: "SELECT id FROM users WHERE id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT id FROM wallets WHERE user_id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(3)}},
		{contains: "UPDATE wallets SET balance=balance+?", err: errors.New("accounting write failed")},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	if completeBlockchainTopup(tx, blockchain.PurchaseIntent{ID: 7, UserID: 42}) == nil {
		t.Fatal("failed accounting activated top-up")
	}
	tx.Rollback()
	if fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("unsafe transaction outcome")
	}
}

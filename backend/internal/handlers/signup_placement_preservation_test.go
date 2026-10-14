package handlers

import (
	"database/sql"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"testing"
)

func TestBlockchainSignupsNeverReassignSiblingPlacementRows(t *testing.T) {
	// Two users can register through the same referrer in the same second.
	// Neither call may select, overwrite or delete the other user's placement;
	// the new row IDs must remain the IDs archived with each purchase.
	fixture := &commissionSQLFixture{}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, row := range []int64{17, 18} {
		ids := []uint64{uint64(row)}
		if err := correctLegacySignupPlacement(tx, true, 5, row, &ids); err != nil {
			t.Fatal(err)
		}
		if len(ids) != 1 || ids[0] != uint64(row) {
			t.Fatal("signup placement identity changed")
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if !fixture.committed {
		t.Fatal("placements were not preserved")
	}
}

func TestLegacySingleSignupCorrectionStillReadsExistingPlacement(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "WHERE virtual_id = ? AND created_at", args: []driver.Value{int64(5), int64(17)}, columns: []string{"id", "user_id", "virtual_id", "parent_id"}, values: []driver.Value{int64(17), int64(42), int64(5), int64(5)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ids := []uint64{17}
	if err := correctLegacySignupPlacement(tx, false, 5, 17, &ids); err != nil {
		t.Fatal(err)
	}
	if len(fixture.steps) != 0 || len(ids) != 1 || ids[0] != 17 {
		t.Fatal("legacy single signup changed")
	}
}

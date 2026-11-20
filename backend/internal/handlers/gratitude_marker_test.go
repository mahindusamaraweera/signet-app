package handlers

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/jmoiron/sqlx"
)

// A gratitude signup must leave exactly one row for the sale (the buyer's),
// so the sponsor's active-sale count isn't inflated by ParentFind's marker.
func TestBlockchainSignupDropsSponsorGratitudeMarker(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		// 664 is the sponsor's marker; the delete is guarded to that shape.
		{contains: "AND node = 'gratitude' AND user_id = ? AND virtual_id = ? AND parent_id = ?", args: []driver.Value{int64(664), int64(471), int64(471), int64(471)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ids := []uint64{664, 665}
	if err := correctLegacySignupPlacement(tx, true, 471, 665, &ids); err != nil {
		t.Fatal(err)
	}
	if len(fixture.steps) != 0 {
		t.Fatalf("missing marker SQL: %d", len(fixture.steps))
	}
	// The buyer's own row (665) is never deleted and stays recorded.
	if len(ids) != 1 || ids[0] != 665 {
		t.Fatalf("created ids = %v, want [665]", ids)
	}
}

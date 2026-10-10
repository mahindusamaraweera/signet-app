package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/tree"
)

func TestPlacementIntermediateWriteUsesCallerTransaction(t *testing.T) {
	failed := errors.New("super-placement read failed")
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "COUNT(*) FROM user_parents", columns: []string{"count"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT id, user_id, virtual_id, parent_id, node FROM user_parents", columns: []string{"id", "user_id", "virtual_id", "parent_id", "node"}, values: []driver.Value{int64(9), int64(8), int64(5), int64(1), "active"}},
		{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"active"}},
		{contains: "INSERT INTO user_parents", insertID: 77, args: []driver.Value{int64(8), int64(5), int64(5), "gratitude"}},
		{contains: "COUNT(*) FROM user_parents", err: failed},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "mysql")
	defer db.Close()
	// A one-connection pool would deadlock if placement opened a second database
	// connection instead of using the supplied transaction.
	db.SetMaxOpenConns(1)
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	var ids []uint64
	_, err = tree.ParentFind(tx, 8, 1, 42, 1, &ids)
	if !errors.Is(err, failed) || len(ids) != 1 || ids[0] != 77 {
		t.Fatalf("wrong placement failure/write evidence: %v %v", err, ids)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("placement did not stay in caller rollback boundary")
	}
}

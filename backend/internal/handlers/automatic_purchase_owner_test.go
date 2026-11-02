package handlers

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func TestAutomaticPurchaseRejectsOwnerDriftBeforeRemoteDebit(t *testing.T) {
	for _, scenario := range []string{"intent owner", "package owner", "missing placement"} {
		t.Run(scenario, func(t *testing.T) {
			intentOwner := int64(42)
			if scenario == "intent owner" {
				intentOwner = 43
			}
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
				{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"pending"}},
				{contains: "SELECT id,user_id,user_package_id", columns: []string{"id", "user_id", "user_package_id", "package_price", "status"}, values: []driver.Value{int64(1), intentOwner, int64(9), "1000", "awaiting_deposit"}},
			}}
			if scenario == "package owner" {
				fixture.steps = append(fixture.steps, commissionSQLStep{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(43)}, args: []driver.Value{int64(9)}})
			}
			if scenario == "missing placement" {
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(42)}, args: []driver.Value{int64(9)}},
					commissionSQLStep{contains: "SELECT id FROM user_parents", err: sql.ErrNoRows, args: []driver.Value{int64(42)}},
				)
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			// No gateway is provided: reaching remote execution would panic rather
			// than silently passing this pre-debit ownership regression.
			err := syncAutomaticPurchase(&app.Deps{DB: db}, 1)
			expected := blockchain.ErrUnavailable
			if scenario == "missing placement" {
				expected = errPurchasePlacementRequired
			}
			if !errors.Is(err, expected) || fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
				t.Fatalf("unsafe ownership boundary: %v", err)
			}
		})
	}
}

func TestFrozenPackageMismatchStopsBeforeRemoteDebit(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(461)}},
		{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"pending"}},
		{contains: "SELECT id,user_id,user_package_id", columns: []string{"id", "user_id", "user_package_id", "package_price", "package_id", "package_snapshot_id", "status"}, values: []driver.Value{int64(2), int64(461), int64(496), "1000", int64(3), int64(3), "awaiting_deposit"}},
		{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(461)}},
		{contains: "SELECT CAST(package AS UNSIGNED)", columns: []string{"package"}, values: []driver.Value{int64(14)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	err := syncAutomaticPurchase(&app.Deps{DB: db}, 2)
	if err == nil || !strings.Contains(err.Error(), "package changed") || fixture.committed || !fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatalf("unsafe debit: %v", err)
	}
}

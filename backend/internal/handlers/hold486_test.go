package handlers

import (
	"database/sql"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
	"time"
)

func hold486Fixture(saved string, held bool) *commissionSQLFixture {
	us, ps := "active", "active"
	if held {
		us = "inactive"
		ps = "deactive"
	}
	rows := [][]driver.Value{}
	if saved != "" {
		rows = append(rows, []driver.Value{saved})
	}
	hash := "0x" + strings.Repeat("f", 64)
	return &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT uuid,status FROM users", columns: []string{"uuid", "status"}, values: []driver.Value{Hold486UUID, us}},
		{contains: "SELECT user_id,package,status,sale,earn", columns: []string{"user_id", "package", "status", "sale", "earn"}, values: []driver.Value{int64(486), "3", ps, "first", "0"}},
		{contains: "SELECT COUNT(*) FROM package_purchase_intents", columns: []string{"count"}, values: []driver.Value{int64(1)}},
		{contains: "FROM blockchain_commission_outbox", columns: []string{"id", "beneficiary_user_id", "commission_kind", "commission_usd", "status", "transfer_tx_hash"}, rowValues: [][]driver.Value{
			{int64(123), int64(471), "gratitude", "200", "confirmed", hash},
			{int64(124), int64(377), "direct_team", "500", "confirmed", hash},
			{int64(125), int64(1), "direct_team", "500", "accounting_only", nil},
			{int64(126), int64(377), "leadership", "50", "confirmed", hash},
			{int64(127), int64(1), "leadership", "50", "accounting_only", nil},
			{int64(128), int64(378), "executive", "50", "confirmed", hash},
		}},
		{contains: "SELECT evidence FROM placement_correction_holds", columns: []string{"evidence"}, rowValues: rows},
	}}
}
func TestHold486DryRunNoWrites(t *testing.T) {
	f := hold486Fixture("", false)
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	r, err := Hold486(db, false, "", strings.Repeat("s", 32), time.Now())
	if err != nil || r["status"] != "dry_run" || f.committed || !f.rolledBack || len(f.steps) != 0 {
		t.Fatalf("%v %v", r, err)
	}
}
func TestHold486ExecuteAndRetryOnlyHoldsSource(t *testing.T) {
	f := hold486Fixture("", false)
	var evidence string
	f.steps = append(f.steps, commissionSQLStep{contains: "INSERT INTO placement_correction_holds", checkArgs: func(a []driver.NamedValue) error { evidence = a[1].Value.(string); return nil }}, commissionSQLStep{contains: "UPDATE users SET status='inactive'"}, commissionSQLStep{contains: "UPDATE user_packages SET status='deactive'"})
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	r, err := Hold486(db, true, "Approved placement correction for source 486", strings.Repeat("s", 32), time.Now())
	if err != nil || r["signature"] == nil || !f.committed || len(f.steps) != 0 {
		t.Fatalf("%v %v", r, err)
	}
	f.steps = hold486Fixture(evidence, true).steps
	if _, err = Hold486(db, true, "ignored repeat reason", strings.Repeat("s", 32), time.Now()); err != nil || len(f.steps) != 0 {
		t.Fatalf("retry %v", err)
	}
}
func TestHold486RejectsChangedCommissionAndRollsBackFailedHold(t *testing.T) {
	f := hold486Fixture("", false)
	f.steps[3].rowValues[0][2] = "direct_team"
	f.steps = f.steps[:4]
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	if _, err := Hold486(db, true, "Approved correction", strings.Repeat("s", 32), time.Now()); err == nil || f.committed {
		t.Fatal("changed role accepted")
	}
	db.Close()
	f = hold486Fixture("", false)
	f.steps = append(f.steps, commissionSQLStep{contains: "INSERT INTO placement_correction_holds"}, commissionSQLStep{contains: "UPDATE users", err: sql.ErrConnDone})
	db = sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	if _, err := Hold486(db, true, "Approved correction", strings.Repeat("s", 32), time.Now()); err == nil || f.committed || !f.rolledBack {
		t.Fatal("partial hold committed")
	}
}

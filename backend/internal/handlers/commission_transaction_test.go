package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

// This scripted SQL driver checks call ordering and transaction propagation.
// It does not substitute for real MySQL atomicity/concurrency testing.
type commissionSQLStep struct {
	checkArgs func([]driver.NamedValue) error
	contains  string
	columns   []string
	values    []driver.Value
	rowValues [][]driver.Value
	args      []driver.Value
	insertID  int64
	err       error
}
type commissionSQLFixture struct {
	steps                 []commissionSQLStep
	committed, rolledBack bool
}

func (f *commissionSQLFixture) Connect(context.Context) (driver.Conn, error) { return f, nil }
func (f *commissionSQLFixture) Driver() driver.Driver                        { return commissionSQLDriver{} }

type commissionSQLDriver struct{}

func (commissionSQLDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }
func (f *commissionSQLFixture) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (f *commissionSQLFixture) Close() error              { return nil }
func (f *commissionSQLFixture) Begin() (driver.Tx, error) { return f, nil }
func (f *commissionSQLFixture) Commit() error             { f.committed = true; return nil }
func (f *commissionSQLFixture) Rollback() error           { f.rolledBack = true; return nil }
func (f *commissionSQLFixture) next(query string, args []driver.NamedValue) (commissionSQLStep, error) {
	if len(f.steps) == 0 {
		return commissionSQLStep{}, errors.New("unexpected SQL: " + query)
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	if !strings.Contains(query, step.contains) {
		return step, errors.New("unexpected SQL order: " + query)
	}
	if step.checkArgs != nil {
		if err := step.checkArgs(args); err != nil {
			return step, err
		}
	}
	if step.args != nil {
		if len(args) != len(step.args) {
			return step, errors.New("wrong arguments count")
		}
		for i, expected := range step.args {
			if args[i].Value != expected {
				return step, errors.New("changed commission evidence argument")
			}
		}
	}
	return step, step.err
}
func (f *commissionSQLFixture) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := f.next(query, args)
	return commissionSQLResult(step.insertID), err
}
func (f *commissionSQLFixture) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := f.next(query, args)
	return &commissionSQLRows{columns: step.columns, values: step.values, rowValues: step.rowValues}, err
}

type commissionSQLResult int64

func (r commissionSQLResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r commissionSQLResult) RowsAffected() (int64, error) { return 1, nil }

type commissionSQLRows struct {
	columns   []string
	values    []driver.Value
	rowValues [][]driver.Value
	rowIndex  int
	read      bool
}

func (r *commissionSQLRows) Columns() []string { return r.columns }
func (r *commissionSQLRows) Close() error      { return nil }
func (r *commissionSQLRows) Next(dest []driver.Value) error {
	if r.rowValues != nil {
		if r.rowIndex >= len(r.rowValues) {
			return io.EOF
		}
		copy(dest, r.rowValues[r.rowIndex])
		r.rowIndex++
		return nil
	}

	if r.read {
		return io.EOF
	}
	r.read = true
	copy(dest, r.values)
	return nil
}

func TestCommissionRecordUsesCreditTransactionAndExactEarnLog(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "outbox failure rollback"}[reject], func(t *testing.T) {
			fixture := &commissionSQLFixture{steps: []commissionSQLStep{
				{contains: "SELECT id FROM wallets", columns: []string{"id"}, values: []driver.Value{int64(9)}},
				{contains: "UPDATE wallets"},
				{contains: "INSERT INTO earn_logs", insertID: 17},
				{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(5), int64(17), int64(42), "50", "Leadership Bonus", "leadership"}},
			}}
			if reject {
				fixture.steps[3].err = errors.New("outbox unavailable")
			} else {
				fixture.steps = append(fixture.steps,
					commissionSQLStep{contains: "COUNT(*) FROM global_share_wallets", columns: []string{"count"}, values: []driver.Value{int64(1)}},
					commissionSQLStep{contains: "SELECT id, balance, max_out", columns: []string{"id", "balance", "max_out"}, values: []driver.Value{int64(8), float64(0), float64(1000)}},
					commissionSQLStep{contains: "UPDATE global_share_wallets"},
					commissionSQLStep{contains: "INSERT INTO global_share_wallets_log"},
					commissionSQLStep{contains: "SELECT up.id", columns: []string{"id", "earn", "price"}, values: []driver.Value{int64(4), "0", int64(100)}},
					commissionSQLStep{contains: "UPDATE user_packages SET earn"})
			}
			db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
			defer db.Close()
			tx, err := db.Beginx()
			if err != nil {
				t.Fatal(err)
			}
			err = creditWalletTx(tx, 42, 50, "Leadership Bonus", purchaseCommissionRecorder(5, "leadership"))
			if reject {
				if err == nil || err.Error() != "outbox unavailable" {
					t.Fatalf("expected outbox rejection, got %v", err)
				}
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
				if fixture.committed || !fixture.rolledBack {
					t.Fatal("failed evidence was committed")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				if !fixture.committed || fixture.rolledBack {
					t.Fatal("credit did not share transaction")
				}
			}
			if len(fixture.steps) != 0 {
				t.Fatal("required SQL was not executed")
			}
		})
	}
}

func TestCompanyOverflowCommissionGetsItsOwnEarnLogEvidence(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT id FROM wallets", columns: []string{"id"}, values: []driver.Value{int64(9)}},
		{contains: "UPDATE wallets"},
		{contains: "INSERT INTO earn_logs", insertID: 17},
		{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(5), int64(17), int64(42), "50", "", "direct_team"}},
		{contains: "COUNT(*) FROM global_share_wallets", columns: []string{"count"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT id, balance, max_out", columns: []string{"id", "balance", "max_out"}, values: []driver.Value{int64(8), float64(1000), float64(1000)}},
		{contains: "SELECT up.id", columns: []string{"id", "earn", "price"}, values: []driver.Value{int64(4), "401", int64(100)}},
		{contains: "UPDATE wallets SET balance", args: []driver.Value{float64(50)}},
		{contains: "INSERT INTO earn_logs", insertID: 18},
		{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(5), int64(18), int64(1), "50", "", "direct_team"}},
		{contains: "UPDATE blockchain_commission_outbox SET status='requires_reconciliation'", args: []driver.Value{int64(18)}},
		{contains: "UPDATE blockchain_commission_outbox SET status='accounting_only'", args: []driver.Value{int64(18)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := creditWalletTx(tx, 42, 50, "", purchaseCommissionRecorder(5, "direct_team")); err != nil {
		t.Fatal(err)
	}
	if len(fixture.steps) != 0 {
		t.Fatal("company commission evidence missing")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

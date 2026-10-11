package withdrawalsecurity

import (
	"context"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
)

// Scripted driver proves SQL ordering/transaction propagation, not MySQL isolation.
type sqlStep struct {
	match    string
	values   []driver.Value
	err      error
	check    func([]driver.NamedValue) error
	affected *int64
}
type sqlFixture struct {
	steps                 []sqlStep
	committed, rolledBack bool
}
type fixtureDriver struct{}

func (fixtureDriver) Open(string) (driver.Conn, error)             { return nil, errors.New("use connector") }
func (f *sqlFixture) Connect(context.Context) (driver.Conn, error) { return f, nil }
func (f *sqlFixture) Driver() driver.Driver                        { return fixtureDriver{} }
func (f *sqlFixture) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (f *sqlFixture) Close() error              { return nil }
func (f *sqlFixture) Begin() (driver.Tx, error) { return f, nil }
func (f *sqlFixture) Commit() error             { f.committed = true; return nil }
func (f *sqlFixture) Rollback() error           { f.rolledBack = true; return nil }
func (f *sqlFixture) next(query string, args []driver.NamedValue) (sqlStep, error) {
	if len(f.steps) == 0 {
		return sqlStep{}, errors.New("unexpected SQL: " + query)
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	if !strings.Contains(query, step.match) {
		return step, errors.New("wrong SQL order: " + query)
	}
	if step.check != nil {
		if err := step.check(args); err != nil {
			return step, err
		}
	}
	return step, step.err
}
func (f *sqlFixture) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	step, err := f.next(query, args)
	count := int64(1)
	if step.affected != nil {
		count = *step.affected
	}
	return driver.RowsAffected(count), err
}
func (f *sqlFixture) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := f.next(query, args)
	return &fixtureRows{values: step.values}, err
}

type fixtureRows struct {
	values []driver.Value
	read   bool
}

func (r *fixtureRows) Columns() []string {
	result := make([]string, len(r.values))
	for i := range result {
		result[i] = "value"
	}
	return result
}
func (r *fixtureRows) Close() error { return nil }
func (r *fixtureRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	copy(dest, r.values)
	return nil
}

package migrate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

type uuidSQLStep struct {
	query  string
	values []driver.Value
	exec   bool
}
type uuidSQLFixture struct {
	steps  []uuidSQLStep
	writes int
}

func (f *uuidSQLFixture) Connect(context.Context) (driver.Conn, error) { return f, nil }
func (f *uuidSQLFixture) Driver() driver.Driver                        { return uuidSQLDriver{} }

type uuidSQLDriver struct{}

func (uuidSQLDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }
func (f *uuidSQLFixture) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (f *uuidSQLFixture) Close() error              { return nil }
func (f *uuidSQLFixture) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (f *uuidSQLFixture) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if len(f.steps) == 0 || !strings.Contains(q, f.steps[0].query) {
		return nil, errors.New("unexpected query: " + q)
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	if step.exec {
		return nil, errors.New("unexpected read")
	}
	return &uuidSQLRows{values: step.values}, nil
}
func (f *uuidSQLFixture) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if len(f.steps) == 0 || !f.steps[0].exec || !strings.Contains(q, f.steps[0].query) {
		return nil, errors.New("unexpected write: " + q)
	}
	f.steps = f.steps[1:]
	if len(args) != 3 || args[0].Value != int64(35) || args[1].Value != "035_add_uuid_to_users.sql" {
		return nil, errors.New("adoption attempted to mark another migration")
	}
	f.writes++
	return driver.RowsAffected(1), nil
}

type uuidSQLRows struct {
	values []driver.Value
	done   bool
}

func (r *uuidSQLRows) Columns() []string {
	cols := make([]string, len(r.values))
	if len(cols) == 0 {
		cols = []string{"uuid"}
	}
	return cols
}
func (r *uuidSQLRows) Close() error { return nil }
func (r *uuidSQLRows) Next(dest []driver.Value) error {
	if r.done || len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}
func TestOnlyCompleteExistingUsersUuidMigrationCanBeAdopted(t *testing.T) {
	for _, scenario := range []string{"valid", "absent", "nullable", "not unique", "invalid values"} {
		t.Run(scenario, func(t *testing.T) {
			first := []driver.Value{"char", "NO", int64(36)}
			if scenario == "absent" {
				first = nil
			}
			if scenario == "nullable" {
				first[1] = "YES"
			}
			fixture := &uuidSQLFixture{steps: []uuidSQLStep{{query: "information_schema.columns", values: first}}}
			if scenario != "absent" && scenario != "nullable" {
				unique := int64(1)
				if scenario == "not unique" {
					unique = 0
				}
				fixture.steps = append(fixture.steps, uuidSQLStep{query: "information_schema.statistics", values: []driver.Value{unique}})
				if unique > 0 {
					invalid := int64(0)
					if scenario == "invalid values" {
						invalid = 1
					}
					fixture.steps = append(fixture.steps, uuidSQLStep{query: "uuid NOT REGEXP", values: []driver.Value{invalid}})
				}
			}
			if scenario == "valid" {
				fixture.steps = append(fixture.steps, uuidSQLStep{query: "INSERT INTO schema_migrations", exec: true})
			}
			db := sql.OpenDB(fixture)
			defer db.Close()
			conn, err := db.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			m := &Migrator{conn: conn, out: io.Discard}
			adopted, err := m.adoptExistingUsersUUID(context.Background(), Migration{Version: 35, UpFile: "035_add_uuid_to_users.sql", Checksum: "test"})
			if scenario == "valid" {
				if err != nil || !adopted || fixture.writes != 1 {
					t.Fatal(adopted, err)
				}
			} else {
				if adopted || fixture.writes != 0 {
					t.Fatal("invalid schema was marked complete")
				}
				if scenario != "absent" && err == nil {
					t.Fatal("incomplete schema accepted")
				}
			}
			if len(fixture.steps) != 0 {
				t.Fatal("verification steps skipped")
			}
		})
	}
}

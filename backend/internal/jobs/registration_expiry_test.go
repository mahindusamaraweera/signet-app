package jobs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"io"
	"net/http"
	"net/http/httptest"
	"signet-backend/internal/blockchain"
	"strings"
	"testing"
	"time"
)

type expiryStep struct {
	query   string
	columns []string
	values  []driver.Value
}
type expiryFixture struct {
	steps      []expiryStep
	rolledBack bool
	writes     bool
	execs      []string
	committed  bool
}

func (f *expiryFixture) Connect(context.Context) (driver.Conn, error) { return &expiryConn{f}, nil }
func (f *expiryFixture) Driver() driver.Driver                        { return expiryDriver{f} }

type expiryDriver struct{ f *expiryFixture }

func (d expiryDriver) Open(string) (driver.Conn, error) { return &expiryConn{d.f}, nil }

type expiryConn struct{ f *expiryFixture }

func (c *expiryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *expiryConn) Close() error              { return nil }
func (c *expiryConn) Begin() (driver.Tx, error) { return expiryTx{c.f}, nil }
func (c *expiryConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if len(c.f.steps) == 0 {
		return nil, errors.New("unexpected query")
	}
	step := c.f.steps[0]
	c.f.steps = c.f.steps[1:]
	if !strings.Contains(q, step.query) {
		return nil, errors.New("expiry violated user-first lock order")
	}
	return &expiryRows{columns: step.columns, values: step.values}, nil
}
func (c *expiryConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.f.writes = true
	if len(c.f.execs) > 0 && strings.Contains(query, c.f.execs[0]) {
		c.f.execs = c.f.execs[1:]
		if strings.Contains(query, "INSERT INTO registration_expiries") {
			var seal blockchain.RegistrationExpirySeal
			if len(args) != 4 {
				return nil, errors.New("missing custody evidence")
			}
			value, ok := args[3].Value.(string)
			if !ok || json.Unmarshal([]byte(value), &seal) != nil || seal.UserID != "42" || seal.PlacementLogID != "7" {
				return nil, errors.New("wrong custody evidence")
			}
		}
		return driver.RowsAffected(1), nil
	}
	return nil, errors.New("unexpected expiry mutation")
}

type expiryTx struct{ f *expiryFixture }

func (t expiryTx) Commit() error {
	if t.f.writes && len(t.f.execs) == 0 {
		t.f.committed = true
		return nil
	}
	return errors.New("unexpected expiry commit")
}
func (t expiryTx) Rollback() error { t.f.rolledBack = true; return nil }

type expiryRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func TestExpiryCommitsOnlyAfterExactSealAndRetainsEvidence(t *testing.T) {
	cutoff := time.Now().UTC().Add(-10 * time.Hour)
	fixture := &expiryFixture{steps: []expiryStep{
		{query: "SELECT user_id FROM user_parent_map_logs", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
		{query: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"pending"}},
		{query: "FROM user_parent_map_logs WHERE id=? FOR UPDATE", columns: []string{"user_id", "created_row_ids", "created_at"}, values: []driver.Value{int64(42), "[1]", cutoff.Add(-time.Minute)}},
		{query: "COUNT(*) FROM user_packages", columns: []string{"protected"}, values: []driver.Value{int64(0)}},
		{query: "deposit_seen=TRUE OR status NOT IN", columns: []string{"protected"}, values: []driver.Value{int64(0)}},
	}, execs: []string{"DELETE FROM user_parents WHERE user_id=? AND node='deactive'", "INSERT INTO registration_expiries", "DELETE FROM user_parent_map_logs", "UPDATE package_purchase_intents SET status='expired'"}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integration/registration-expiry/seal" {
			t.Error("read-only clearance used")
		}
		w.Write([]byte(`{"can_expire":true,"seal":{"external_user_id":"42","placement_log_id":"7","sealed_at":"2026-01-01T00:00:00Z","scanned_block":990,"observed_head":1000}}`))
	}))
	defer s.Close()
	g, _ := blockchain.New(s.URL, "test")
	db := sqlx.NewDb(sql.OpenDB(fixture), "mysql")
	defer db.Close()
	if err := expireRegistration(db, g, true, 7, cutoff); err != nil || !fixture.committed || fixture.rolledBack || len(fixture.steps) != 0 || len(fixture.execs) != 0 {
		t.Fatalf("unsafe expiry commit: %v", err)
	}
}

func (r *expiryRows) Columns() []string { return r.columns }
func (r *expiryRows) Close() error      { return nil }
func (r *expiryRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}

func TestExpiryLocksUserBeforePlacementAndRevalidatesOwner(t *testing.T) {
	for _, scenario := range []string{"active account", "changed placement owner", "financial protection", "partial deposit", "pending purchase", "missing custody gateway"} {
		t.Run(scenario, func(t *testing.T) {
			cutoff := time.Now().UTC().Add(-10 * time.Hour)
			owner := int64(42)
			status := "pending"
			if scenario == "active account" {
				status = "active"
			}
			if scenario == "changed placement owner" {
				owner = 43
			}
			fixture := &expiryFixture{steps: []expiryStep{
				{query: "SELECT user_id FROM user_parent_map_logs", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
				{query: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{status}},
				{query: "FROM user_parent_map_logs WHERE id=? FOR UPDATE", columns: []string{"user_id", "created_row_ids", "created_at"}, values: []driver.Value{owner, "[1]", cutoff.Add(-time.Minute)}},
			}}
			if scenario == "financial protection" {
				fixture.steps = append(fixture.steps, expiryStep{query: "COUNT(*) FROM user_packages", columns: []string{"protected"}, values: []driver.Value{int64(1)}})
			}
			if scenario == "partial deposit" || scenario == "pending purchase" || scenario == "missing custody gateway" {
				fixture.steps = append(fixture.steps, expiryStep{query: "COUNT(*) FROM user_packages", columns: []string{"protected"}, values: []driver.Value{int64(0)}})
				protected := int64(1)
				if scenario == "missing custody gateway" {
					protected = 0
				}
				fixture.steps = append(fixture.steps, expiryStep{query: "deposit_seen=TRUE OR status NOT IN ('awaiting_deposit','expired')", columns: []string{"protected"}, values: []driver.Value{protected}})
			}
			raw := sql.OpenDB(fixture)
			defer raw.Close()
			db := sqlx.NewDb(raw, "mysql")
			err := expireRegistration(db, nil, true, 7, cutoff)
			validError := err == nil
			if scenario == "missing custody gateway" {
				validError = errors.Is(err, blockchain.ErrUnavailable)
			}
			if !validError || fixture.writes || !fixture.rolledBack || len(fixture.steps) != 0 {
				t.Fatalf("unsafe expiry boundary: %v", err)
			}
		})
	}
}

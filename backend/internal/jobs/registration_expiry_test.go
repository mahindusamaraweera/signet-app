package jobs

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/blockchain"
)

type expiryStep struct {
	query   string
	columns []string
	values  []driver.Value
}
type expiryFixture struct {
	steps                         []expiryStep
	execs                         []string
	committed, rolledBack, writes bool
	failWrite                     string
	deleteRows                    int64
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
		return nil, fmt.Errorf("unexpected query: %s", q)
	}
	step := c.f.steps[0]
	c.f.steps = c.f.steps[1:]
	if !strings.Contains(q, step.query) {
		return nil, fmt.Errorf("want %s; got %s", step.query, q)
	}
	return &expiryRows{columns: step.columns, values: step.values}, nil
}
func (c *expiryConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	c.f.writes = true
	if c.f.failWrite != "" && strings.Contains(q, c.f.failWrite) {
		return nil, errors.New("simulated write failure")
	}
	if len(c.f.execs) == 0 || !strings.Contains(q, c.f.execs[0]) {
		return nil, fmt.Errorf("unexpected mutation: %s", q)
	}
	c.f.execs = c.f.execs[1:]
	if strings.Contains(q, "INSERT INTO registration_expiries") {
		var seal blockchain.RegistrationExpirySeal
		if len(args) != 4 {
			return nil, errors.New("missing seal")
		}
		value, ok := args[3].Value.(string)
		if !ok || json.Unmarshal([]byte(value), &seal) != nil || seal.UserID != "42" || seal.PlacementLogID != "7" {
			return nil, errors.New("wrong seal")
		}
	}
	if strings.Contains(q, "INSERT INTO registration_account_deletions") {
		if len(args) != 9 || args[0].Value != int64(42) || args[3].Value != "signet-app:42" {
			return nil, errors.New("missing deletion evidence")
		}
		for _, arg := range args[4:8] {
			value, ok := arg.Value.(string)
			if !ok || !json.Valid([]byte(value)) || strings.Contains(value, "password") {
				return nil, errors.New("invalid audit snapshot")
			}
		}
	}
	if strings.HasPrefix(q, "DELETE FROM") {
		if len(args) != 1 || (args[0].Value != int64(42) && args[0].Value != "expired@example.test") {
			return nil, errors.New("deletion crossed ownership")
		}
	}
	if strings.Contains(q, "DELETE FROM users") {
		return driver.RowsAffected(c.f.deleteRows), nil
	}
	return driver.RowsAffected(1), nil
}

type expiryTx struct{ f *expiryFixture }

func (t expiryTx) Commit() error {
	if !t.f.writes || len(t.f.execs) != 0 {
		return errors.New("unexpected commit")
	}
	t.f.committed = true
	return nil
}
func (t expiryTx) Rollback() error { t.f.rolledBack = true; return nil }

type expiryRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *expiryRows) Columns() []string { return r.columns }
func (r *expiryRows) Close() error      { return nil }
func (r *expiryRows) Next(dest []driver.Value) error {
	if r.done || r.values == nil {
		return io.EOF
	}
	r.done = true
	copy(dest, r.values)
	return nil
}

func expiryBaseFixture(cutoff time.Time) *expiryFixture {
	return &expiryFixture{deleteRows: 1, steps: []expiryStep{
		{"SELECT user_id FROM user_parent_map_logs", []string{"user_id"}, []driver.Value{int64(42)}},
		{"SELECT status,role,email", []string{"status", "role", "email", "created_at", "email_verified_at"}, []driver.Value{"pending", "user", "expired@example.test", cutoff.Add(-time.Hour), cutoff.Add(-time.Hour)}},
		{"registration_expiry_enrollments WHERE user_id=? FOR UPDATE", []string{"policy_version"}, []driver.Value{"new_unpaid_v1"}},
		{"FROM user_parent_map_logs WHERE id=? FOR UPDATE", []string{"user_id", "created_row_ids", "created_at"}, []driver.Value{int64(42), "[1,999]", cutoff.Add(-time.Minute)}},
		{"COUNT(*) FROM user_packages", []string{"protected"}, []driver.Value{int64(0)}},
		{"COUNT(*) FROM earn_logs", []string{"protected"}, []driver.Value{int64(0)}},
		{"deposit_seen=TRUE OR available_usdt<>0 OR status NOT IN ('awaiting_deposit','expired')", []string{"total", "protected"}, []driver.Value{int64(1), int64(0)}},
	}}
}
func expiryAddDeletion(f *expiryFixture, withPlacement bool) {
	f.steps = append(f.steps,
		expiryStep{"SELECT id,name,email", []string{"id", "email"}, []driver.Value{int64(42), "expired@example.test"}},
		expiryStep{"SELECT id,package,status", []string{"id", "package", "status"}, []driver.Value{int64(8), "1000", "pending"}},
		expiryStep{"SELECT id,user_package_id,package_price", []string{"id", "package_price", "status"}, []driver.Value{int64(9), "1000", "awaiting_deposit"}},
		expiryStep{"SELECT id,user_id,virtual_id,parent_id", []string{"id", "user_id", "node"}, []driver.Value{int64(1), int64(42), "deactive"}},
	)
	f.execs = []string{"INSERT INTO registration_account_deletions"}
	if withPlacement {
		f.execs = append(f.execs, "INSERT INTO registration_expiries")
	}
	f.execs = append(f.execs, "UPDATE package_purchase_intents SET status='expired'")
	for _, table := range []string{"referral_codes", "user_parent_map_logs", "user_parents", "user_packages", "wallets", "global_share_wallets", "user_secret_keys", "sessions", "withdrawal_security_challenges", "withdrawal_authenticator_enrollments", "withdrawal_totp_counters", "withdrawal_security_profiles", "withdrawal_security_addresses", "withdrawal_security_devices", "tier_account_link_requests"} {
		f.execs = append(f.execs, "DELETE FROM "+table+" WHERE user_id=?")
	}
	f.execs = append(f.execs, "DELETE FROM password_reset WHERE email=?", "DELETE FROM password_reset_tokens WHERE email=?", "UPDATE pending_email_registrations SET status='account_expired'", "DELETE FROM users WHERE id=? AND status='pending' AND role='user'")
}
func expiryTestGateway(t *testing.T, outcome string, sealID uint64) (*blockchain.Gateway, func()) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if outcome == "unavailable" {
			w.WriteHeader(503)
			return
		}
		switch r.URL.Path {
		case "/api/v1/integration/wallets":
			amount := "0"
			if outcome == "balance" {
				amount = "0.000000000000000001"
			}
			fmt.Fprintf(w, `{"address":"0x1111111111111111111111111111111111111111","chain_id":56,"asset":"USDT","available":"%s","provisioning_status":"ready","balances":{"USDT":{"available":"%s","locked":"0","pending_withdrawal":"0"},"SIGNET":{"available":"0","locked":"0","pending_withdrawal":"0"}}}`, amount, amount)
		case "/api/v1/integration/registration-expiry/seal":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["external_user_id"] != "42" || body["placement_log_id"] != fmt.Sprint(sealID) {
				t.Error("seal identity mismatch")
			}
			if outcome == "refused" {
				w.Write([]byte(`{"can_expire":false,"seal":null}`))
				return
			}
			sealedAt := time.Now().UTC()
			if outcome == "stale seal" {
				sealedAt = sealedAt.Add(-time.Hour)
			}
			fmt.Fprintf(w, `{"can_expire":true,"seal":{"external_user_id":"42","placement_log_id":"%d","sealed_at":"%s","scanned_block":990,"observed_head":1000}}`, sealID, sealedAt.Format(time.RFC3339))
		case "/api/v1/integration/deposits":
			if outcome == "late deposit" {
				w.Write([]byte(`{"deposits":[{"id":1,"amount":"0.01","status":"detected"}]}`))
				return
			}
			if outcome == "malformed deposits" {
				w.Write([]byte(`{"deposits":null}`))
				return
			}
			w.Write([]byte(`{"deposits":[]}`))
		default:
			t.Errorf("unexpected API %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	g, err := blockchain.New(s.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	return g, s.Close
}
func TestExpiryDeletesNewUnpaidUserOnlyAfterAuditAndSeal(t *testing.T) {
	cutoff := time.Now().UTC().Add(-10 * time.Hour)
	f := expiryBaseFixture(cutoff)
	expiryAddDeletion(f, true)
	g, close := expiryTestGateway(t, "clear", 7)
	defer close()
	db := sqlx.NewDb(sql.OpenDB(f), "mysql")
	defer db.Close()
	if err := expireRegistration(db, g, true, 7, cutoff); err != nil || !f.committed || f.rolledBack || len(f.steps) != 0 || len(f.execs) != 0 {
		t.Fatalf("unsafe expiry: %v steps=%d writes=%d", err, len(f.steps), len(f.execs))
	}
}

func TestExpirySchedulerRequiresEnrollmentAndRotatesCheckedAccounts(t *testing.T) {
	cutoff := time.Now().UTC().Add(-10 * time.Hour)
	f := expiryBaseFixture(cutoff)
	f.steps = f.steps[:3]
	f.steps[2].values = nil // Defense in depth if a candidate loses enrollment.
	f.steps = append([]expiryStep{{query: "JOIN registration_expiry_enrollments e ON e.user_id=u.id", columns: []string{"user_id", "log_id"}, values: []driver.Value{int64(42), int64(7)}}}, f.steps...)
	f.execs = []string{"UPDATE registration_expiry_enrollments SET last_checked_at=NOW()"}
	db := sqlx.NewDb(sql.OpenDB(f), "mysql")
	defer db.Close()
	if err := RunRegistrationExpiry(db, nil, true); err != nil || f.committed || !f.rolledBack || len(f.execs) != 0 || len(f.steps) != 0 {
		t.Fatalf("unsafe candidate scope: %v", err)
	}
}
func TestExpiryNeverDeletesExistingOrProtectedUsers(t *testing.T) {
	for _, scenario := range []string{"existing user", "already deleted", "active user", "admin", "changed owner", "before deadline", "balance", "financial history", "partial deposit", "pending purchase", "blockchain disabled with deposit", "blockchain disabled without intent", "missing gateway"} {
		t.Run(scenario, func(t *testing.T) {
			cutoff := time.Now().UTC().Add(-10 * time.Hour)
			f := expiryBaseFixture(cutoff)
			enabled := true
			switch scenario {
			case "existing user":
				f.steps = f.steps[:3]
				f.steps[2].values = nil
			case "already deleted":
				f.steps = f.steps[:2]
				f.steps[1].values = nil
			case "active user":
				f.steps = f.steps[:2]
				f.steps[1].values[0] = "active"
			case "admin":
				f.steps = f.steps[:2]
				f.steps[1].values[1] = "company"
			case "changed owner":
				f.steps = f.steps[:4]
				f.steps[3].values[0] = int64(43)
			case "before deadline":
				f.steps = f.steps[:4]
				f.steps[3].values[2] = cutoff.Add(time.Second)
			case "balance":
				f.steps = f.steps[:5]
				f.steps[4].values[0] = int64(1)
			case "financial history":
				f.steps = f.steps[:6]
				f.steps[5].values[0] = int64(1)
			case "partial deposit", "pending purchase", "blockchain disabled with deposit":
				f.steps[6].values[1] = int64(1)
				if scenario == "blockchain disabled with deposit" {
					enabled = false
				}
			case "blockchain disabled without intent":
				enabled = false
				f.steps[6].values[0] = int64(0)
			}
			db := sqlx.NewDb(sql.OpenDB(f), "mysql")
			defer db.Close()
			err := expireRegistration(db, nil, enabled, 7, cutoff)
			valid := err == nil
			if scenario == "missing gateway" || scenario == "blockchain disabled without intent" {
				valid = errors.Is(err, blockchain.ErrUnavailable)
			}
			if !valid || f.writes || !f.rolledBack || len(f.steps) != 0 {
				t.Fatalf("unsafe protection: %v steps=%d", err, len(f.steps))
			}
		})
	}
}
func TestExpiryRemoteFailureAndDepositsNeverDelete(t *testing.T) {
	for _, outcome := range []string{"unavailable", "balance", "refused", "late deposit", "malformed deposits", "stale seal"} {
		t.Run(outcome, func(t *testing.T) {
			cutoff := time.Now().UTC().Add(-10 * time.Hour)
			f := expiryBaseFixture(cutoff)
			g, close := expiryTestGateway(t, outcome, 7)
			defer close()
			db := sqlx.NewDb(sql.OpenDB(f), "mysql")
			defer db.Close()
			err := expireRegistration(db, g, true, 7, cutoff)
			if (outcome == "unavailable" || outcome == "malformed deposits" || outcome == "stale seal") && err == nil {
				t.Fatal("failure not reported")
			}
			if f.writes || f.committed || !f.rolledBack || len(f.steps) != 0 {
				t.Fatalf("unsafe remote failure: %v", err)
			}
		})
	}
}
func TestExpiryAuditFailureAndLostDeleteRollBack(t *testing.T) {
	for _, failure := range []string{"audit", "delete failure", "lost delete"} {
		t.Run(failure, func(t *testing.T) {
			cutoff := time.Now().UTC().Add(-10 * time.Hour)
			f := expiryBaseFixture(cutoff)
			expiryAddDeletion(f, true)
			if failure == "audit" {
				f.failWrite = "INSERT INTO registration_account_deletions"
			}
			if failure == "delete failure" {
				f.failWrite = "DELETE FROM users"
			}
			if failure == "lost delete" {
				f.deleteRows = 0
			}
			g, close := expiryTestGateway(t, "clear", 7)
			defer close()
			db := sqlx.NewDb(sql.OpenDB(f), "mysql")
			defer db.Close()
			if err := expireRegistration(db, g, true, 7, cutoff); err == nil || f.committed || !f.rolledBack {
				t.Fatalf("failed deletion committed: %v", err)
			}
		})
	}
}
func TestExpiryNewUserWithoutPackageUsesStableCustodyReference(t *testing.T) {
	cutoff := time.Now().UTC().Add(-10 * time.Hour)
	f := expiryBaseFixture(cutoff)
	f.steps = f.steps[1:]
	f.steps[2] = expiryStep{"SELECT COUNT(*) FROM user_parent_map_logs WHERE user_id=?", []string{"count"}, []driver.Value{int64(0)}}
	f.steps[5].values[0] = int64(0)
	expiryAddDeletion(f, false)
	g, close := expiryTestGateway(t, "clear", uint64(1)<<63|42)
	defer close()
	db := sqlx.NewDb(sql.OpenDB(f), "mysql")
	defer db.Close()
	if err := expireUnpaidAccount(db, g, true, 0, 42, cutoff); err != nil || !f.committed || len(f.steps) != 0 || len(f.execs) != 0 {
		t.Fatalf("account-only expiry: %v", err)
	}
}

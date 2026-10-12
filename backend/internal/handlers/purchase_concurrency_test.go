package handlers

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/config"
	"signet-backend/internal/models"
)

// Models the user FOR UPDATE boundary while reusing all activation SQL
// assertions. This unit test does not replace live MySQL/blockchain testing.
type activationLockDB struct {
	fixture   *commissionSQLFixture
	mu        sync.Mutex
	committed bool
	waiting   chan struct{}
}

func (d *activationLockDB) Connect(context.Context) (driver.Conn, error) {
	return &activationLockConn{db: d}, nil
}
func (d *activationLockDB) Driver() driver.Driver { return commissionSQLDriver{} }

type activationLockConn struct {
	db     *activationLockDB
	locked bool
	tx     bool
}

func (c *activationLockConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *activationLockConn) Close() error              { return nil }
func (c *activationLockConn) Begin() (driver.Tx, error) { c.tx = true; return c, nil }
func (c *activationLockConn) Commit() error {
	c.db.committed = true
	c.db.fixture.committed = true
	c.locked = false
	c.tx = false
	c.db.mu.Unlock()
	return nil
}
func (c *activationLockConn) Rollback() error {
	if c.locked {
		c.locked = false
		c.db.mu.Unlock()
	}
	c.tx = false
	return nil
}
func (c *activationLockConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if !c.tx && strings.Contains(q, "SELECT user_id FROM package_purchase_intents") {
		return &commissionSQLRows{columns: []string{"user_id"}, values: []driver.Value{int64(42)}}, nil
	}
	if strings.Contains(q, "SELECT status FROM users") {
		if !strings.Contains(q, "FOR UPDATE") {
			return nil, errors.New("missing activation serialization lock")
		}
		select {
		case c.db.waiting <- struct{}{}:
		default:
		}
		c.db.mu.Lock()
		c.locked = true
		if c.db.committed {
			return &commissionSQLRows{columns: []string{"status"}, values: []driver.Value{"active"}}, nil
		}
	}
	if !c.locked {
		return nil, errors.New("activation read outside owner lock")
	}
	if c.db.committed && strings.Contains(q, "SELECT id,user_id,user_package_id") {
		return &commissionSQLRows{columns: []string{"id"}, read: true}, nil
	}
	return c.db.fixture.QueryContext(ctx, q, args)
}
func (c *activationLockConn) ExecContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if !c.locked {
		return nil, errors.New("activation write outside owner lock")
	}
	return c.db.fixture.ExecContext(ctx, q, args)
}

func TestManualAndAutomaticActivationOverlapWithoutDuplicateSettlement(t *testing.T) {
	fixture := activationCapFixture()
	fixture.steps = fixture.steps[1:]
	connector := &activationLockDB{fixture: fixture, waiting: make(chan struct{}, 2)}
	db := sqlx.NewDb(sql.OpenDB(connector), "commission-fixture")
	defer db.Close()
	db.SetMaxOpenConns(2)
	remoteEntered, finishRemote := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(remoteEntered)
			<-finishRemote
		}
		w.Write([]byte(`{"status":"completed","available_usdt":"20","deposit_seen":true,"order_id":"remote-1","swap_confirmed":true,"vesting_allocated":true}`))
	}))
	defer server.Close()
	gateway, err := blockchain.New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	d := &app.Deps{DB: db, Blockchain: gateway}
	results := make(chan error, 2)
	go func() { results <- syncAutomaticPurchase(d, 1) }()
	select {
	case <-remoteEntered:
	case <-time.After(3 * time.Second):
		close(finishRemote)
		t.Fatal("automatic activation did not reach settlement")
	}
	<-connector.waiting
	go func() { results <- syncAutomaticPurchaseForUser(d, 1, 42) }()
	select {
	case <-connector.waiting:
	case <-time.After(3 * time.Second):
		close(finishRemote)
		t.Fatal("manual request did not overlap")
	}
	close(finishRemote)
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("activation did not finish")
		}
	}
	if calls.Load() != 1 || !fixture.committed || len(fixture.steps) != 0 {
		t.Fatal("overlapping activation duplicated settlement or commission writes")
	}
}

func TestManualActivationRejectsForeignSelectedPurchase(t *testing.T) {
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT * FROM users WHERE id", columns: []string{"id", "role", "status", "password"}, values: []driver.Value{int64(42), "user", "pending", "test-hash"}},
		{contains: "SELECT id FROM package_purchase_intents WHERE user_id=? AND id=?", args: []driver.Value{int64(42), int64(999)}, err: sql.ErrNoRows},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("foreign purchase reached custody") }))
	defer server.Close()
	gateway, _ := blockchain.New(server.URL, "test")
	d := &app.Deps{DB: db, Blockchain: gateway, Cfg: &config.Config{BlockchainEnabled: true}, Auth: auth.New(db, "test", time.Hour, false)}
	r := authenticatedRecoveryRequest(t, d, &models.User{ID: 42, Role: "user", Status: "pending", Password: "test-hash"}, "POST", "/blockchain/purchase/sync", `{"purchase_id":"999"}`)
	w := httptest.NewRecorder()
	d.Auth.RequireAuth(http.HandlerFunc(purchaseSyncHandler(d))).ServeHTTP(w, r)
	if w.Code != 404 || len(fixture.steps) != 0 || fixture.committed {
		t.Fatalf("foreign selection accepted: %d %s", w.Code, w.Body.String())
	}
}

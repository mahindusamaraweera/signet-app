package handlers

import (
	"database/sql"
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmoiron/sqlx"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func TestAutomaticPurchaseDeniedCapActivatesAndCreditsCompanyOnce(t *testing.T) {
	fixture := activationCapFixture()
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Idempotency-Key") != "signet-app:purchase:1" {
			t.Error("changed durable purchase identity")
		}
		w.Write([]byte(`{"status":"completed","available_usdt":"20","deposit_seen":true,"order_id":"remote-1","swap_confirmed":true,"vesting_allocated":true}`))
	}))
	defer server.Close()
	gateway, err := blockchain.New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := syncAutomaticPurchase(&app.Deps{DB: db, Blockchain: gateway}, 1); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !fixture.committed || fixture.rolledBack || len(fixture.steps) != 0 {
		t.Fatal("company diversion and activation were not committed together")
	}
	// A manual click racing just after the automatic commit, and another
	// worker tick, must both observe completion without any remote replay.
	for _, expectedUserID := range []uint64{42, 0} {
		fixture.steps = []commissionSQLStep{
			{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
			{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"active"}},
			{contains: "SELECT id,user_id,user_package_id", err: sql.ErrNoRows},
		}
		if err := syncAutomaticPurchaseForUser(&app.Deps{DB: db, Blockchain: gateway}, 1, expectedUserID); err != nil {
			t.Fatal(err)
		}
		if calls != 1 || len(fixture.steps) != 0 {
			t.Fatal("completed activation was duplicated")
		}
	}
}

func activationCapFixture() *commissionSQLFixture {
	return &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT user_id FROM package_purchase_intents", columns: []string{"user_id"}, values: []driver.Value{int64(42)}},
		{contains: "SELECT status FROM users", columns: []string{"status"}, values: []driver.Value{"inactive"}},
		{contains: "SELECT id,user_id,user_package_id", columns: []string{"id", "user_id", "user_package_id", "package_price", "commission_user_id", "commission_percent", "package_id", "status"}, values: []driver.Value{int64(1), int64(42), int64(9), "100", int64(77), int64(50), int64(1), "awaiting_deposit"}},
		{contains: "SELECT user_id FROM user_packages", columns: []string{"user_id"}, values: []driver.Value{int64(42)}, args: []driver.Value{int64(9)}},
		{contains: "SELECT id FROM user_parents", columns: []string{"id"}, values: []driver.Value{int64(6)}, args: []driver.Value{int64(42)}},
		{contains: "SELECT up.id,up.user_id", columns: []string{"id", "user_id", "package", "status", "sale", "earn", "price", "commission"}, values: []driver.Value{int64(9), int64(42), "1", "pending", "first", "0", int64(100), int64(50)}},
		{contains: "SELECT id FROM users WHERE id=? FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(77)}, args: []driver.Value{int64(77)}},
		{contains: "SELECT id FROM wallets WHERE user_id=? FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(10)}, args: []driver.Value{int64(77)}},
		{contains: "SUM(p.price * 4)", columns: []string{"cap"}, values: []driver.Value{float64(400)}},
		{contains: "SELECT balance FROM wallets", columns: []string{"balance"}, values: []driver.Value{float64(351)}},
		{contains: "SELECT p.commission", columns: []string{"commission"}, values: []driver.Value{int64(50)}},
		{contains: "SELECT price FROM packages", columns: []string{"price"}, values: []driver.Value{float64(100)}},
		{contains: "SELECT id, virtual_id, node FROM user_parents", columns: []string{"id", "virtual_id", "node"}, values: []driver.Value{int64(6), int64(77), "deactive"}},
		{contains: "UPDATE user_parents SET node"},
		{contains: "UPDATE user_packages SET status='active'"},
		{contains: "UPDATE users SET status='active'"},
		{contains: "SELECT id FROM users WHERE id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT id FROM wallets WHERE user_id=1 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(11)}},
		{contains: "UPDATE wallets SET balance=balance+?", args: []driver.Value{float64(50), int64(11)}},
		{contains: "INSERT INTO earn_logs", insertID: 18},
		{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(1), int64(18), int64(1), "50", "Income cap commission diversion", "direct_team"}},
		{contains: "INSERT INTO package_pools"},
		{contains: "SELECT * FROM users", columns: []string{"id"}, values: []driver.Value{int64(42)}},
		{contains: "INSERT INTO income_cap_diversions", args: []driver.Value{int64(77), int64(1), "50"}},
		{contains: "UPDATE package_purchase_intents SET status", args: []driver.Value{"completed", "20", "0.000000000000000000", true, "remote-1", int64(1)}},
	}}
}

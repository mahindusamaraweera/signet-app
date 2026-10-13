package blockchain

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

type walletAuditMapping struct {
	chain   int64
	address string
}
type walletAuditFixture struct {
	rows   map[int64]walletAuditMapping
	writes int
}

func (f *walletAuditFixture) Connect(context.Context) (driver.Conn, error) {
	return walletAuditConn{f}, nil
}
func (f *walletAuditFixture) Driver() driver.Driver { return walletAuditDriver{f} }

type walletAuditDriver struct{ f *walletAuditFixture }

func (d walletAuditDriver) Open(string) (driver.Conn, error) { return walletAuditConn{d.f}, nil }

type walletAuditConn struct{ f *walletAuditFixture }

func (c walletAuditConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c walletAuditConn) Close() error              { return nil }
func (c walletAuditConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c walletAuditConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.HasPrefix(q, "INSERT INTO blockchain_wallet_provisioning_audit") || !strings.HasSuffix(q, "ON DUPLICATE KEY UPDATE source_user_id=source_user_id") || len(args) != 3 {
		return nil, errors.New("audit attempted to modify an account or mapping")
	}
	c.f.writes++
	id := args[0].Value.(int64)
	chain := args[1].Value.(int64)
	address := args[2].Value.(string)
	if _, exists := c.f.rows[id]; exists {
		return driver.RowsAffected(0), nil
	}
	for _, row := range c.f.rows {
		if row.chain == chain && row.address == address {
			return driver.RowsAffected(0), nil
		}
	}
	c.f.rows[id] = walletAuditMapping{chain, address}
	return driver.RowsAffected(1), nil
}
func (c walletAuditConn) QueryContext(_ context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if q != "SELECT chain_id,deposit_address FROM blockchain_wallet_provisioning_audit WHERE source_user_id=?" {
		return nil, errors.New("unexpected audit query")
	}
	row, exists := c.f.rows[args[0].Value.(int64)]
	return &walletAuditRows{row: row, exists: exists}, nil
}

type walletAuditRows struct {
	row    walletAuditMapping
	exists bool
	done   bool
}

func (r *walletAuditRows) Columns() []string { return []string{"chain_id", "deposit_address"} }
func (r *walletAuditRows) Close() error      { return nil }
func (r *walletAuditRows) Next(values []driver.Value) error {
	if !r.exists || r.done {
		return io.EOF
	}
	r.done = true
	values[0] = r.row.chain
	values[1] = r.row.address
	return nil
}

func TestWalletAuditRetryPreservesImmutableSourceMapping(t *testing.T) {
	f := &walletAuditFixture{rows: map[int64]walletAuditMapping{}}
	db := sql.OpenDB(f)
	defer db.Close()
	recorder := SQLWalletAuditRecorder{DB: db}
	original := &Wallet{ChainID: 56, Address: "0xABCDEF1234567890123456789012345678901234"}
	for i := 0; i < 2; i++ {
		if err := recorder.RecordWallet(context.Background(), 42, original); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.rows) != 1 || f.rows[42].address != strings.ToLower(original.Address) {
		t.Fatal("audit duplicated or changed source mapping")
	}
	changed := &Wallet{ChainID: 56, Address: "0x1234567890123456789012345678901234567890"}
	if err := recorder.RecordWallet(context.Background(), 42, changed); err == nil {
		t.Fatal("changed deposit address accepted")
	}
	if err := recorder.RecordWallet(context.Background(), 43, original); err == nil {
		t.Fatal("address assigned to another source user")
	}
	if len(f.rows) != 1 || f.rows[42].address != strings.ToLower(original.Address) {
		t.Fatal("conflict overwrote old identity")
	}
}

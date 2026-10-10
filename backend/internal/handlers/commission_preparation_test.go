package handlers

import (
	"database/sql"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"net/http"
	"net/http/httptest"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"testing"
)

func TestCommissionPreparationPersistsTermsWithoutDeclaringPayment(t *testing.T) {
	quote := `{"signet_price_usdt":"0.125","signet_amount":"400"}`
	address := "0x1234567890123456789012345678901234567890"
	fixture := &commissionSQLFixture{steps: []commissionSQLStep{
		{contains: "SELECT o.id,o.earn_log_id", columns: []string{"id", "earn_log_id", "purchase_intent_id", "buyer_id", "beneficiary_user_id", "commission_kind", "commission_usd"}, values: []driver.Value{int64(7), int64(18), int64(1), int64(42), int64(77), "direct_team", "50"}},
		{contains: "SET status='awaiting_authorization'", args: []driver.Value{"1", "0.125", "400", address, quote, int64(7)}},
	}}
	db := sqlx.NewDb(sql.OpenDB(fixture), "commission-fixture")
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"1","external_outbox_id":"7","external_earn_log_id":"18","external_recipient_id":"77","commission_kind":"direct_team","commission_usd":"50","recipient_address":"` + address + `","status":"awaiting_authorization","quote":` + quote + `}`))
	}))
	defer server.Close()
	g, _ := blockchain.New(server.URL, "test")
	if err := syncCommissionPreparation(&app.Deps{DB: db, Blockchain: g}, 7); err != nil {
		t.Fatal(err)
	}
	if !fixture.committed || len(fixture.steps) != 0 {
		t.Fatal("prepared terms not committed")
	}
}

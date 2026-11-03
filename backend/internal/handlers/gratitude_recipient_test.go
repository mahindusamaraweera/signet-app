package handlers

import (
	"database/sql"
	"database/sql/driver"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
)

func TestFirstGratitudeResolvesPackageLogWithoutDoubleCredit(t *testing.T) {
	for _, tt := range []struct {
		name, node string
		virtual    uint64
		rows       [][]driver.Value
		want       uint64
		bad        bool
	}{
		{"active buyer saved recipient", "active", 132, [][]driver.Value{{int64(132)}}, 132, false},
		{"same node and saved recipient", "gratitude", 132, [][]driver.Value{{int64(132)}}, 132, false},
		{"legacy gratitude", "gratitude", 132, nil, 132, false},
		{"ordinary buyer", "active", 132, nil, 0, false},
		{"conflicting recipient", "gratitude", 133, [][]driver.Value{{int64(132)}}, 0, true},
		{"ambiguous logs", "active", 132, [][]driver.Value{{int64(132)}, {int64(132)}}, 0, true},
		{"invalid recipient", "active", 132, [][]driver.Value{{int64(0)}}, 0, true},
		{"invalid legacy recipient", "gratitude", 0, nil, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.rows == nil {
				tt.rows = [][]driver.Value{}
			}
			f := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "WHERE gratitude_user=? AND user_package=?", columns: []string{"current_user_id"}, rowValues: tt.rows, args: []driver.Value{int64(470), int64(505)}}}}
			db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
			defer db.Close()
			tx, _ := db.Beginx()
			defer tx.Rollback()
			got, err := firstGratitudeRecipient(tx, 470, 505, tt.node, tt.virtual)
			if (err != nil) != tt.bad || got != tt.want {
				t.Fatalf("got %d %v", got, err)
			}
		})
	}
}

// Exercise activation itself: active buyer + saved recipient creates exactly
// one 20% outbox credit before the independent direct commission.
func TestActivationActiveBuyerCreditsSavedGratitudeOnce(t *testing.T) {
	f := activationCapFixture()
	for i, s := range f.steps {
		if strings.Contains(s.contains, "SELECT current_user_id FROM super_parent_logs") {
			f.steps[i].rowValues = [][]driver.Value{{int64(132)}}
			extra := []commissionSQLStep{
				{contains: "SELECT id FROM users WHERE id=? FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(132)}},
				{contains: "SELECT balance FROM wallets", columns: []string{"balance"}, values: []driver.Value{float64(0)}},
				{contains: "SUM(p.price*4)", columns: []string{"cap"}, values: []driver.Value{float64(4000)}},
				{contains: "SELECT id FROM wallets", columns: []string{"id"}, values: []driver.Value{int64(12)}},
				{contains: "UPDATE wallets", args: []driver.Value{float64(20), int64(132)}},
				{contains: "INSERT INTO earn_logs", insertID: 101},
				{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(1), int64(101), int64(132), "20", "", "gratitude"}},
				{contains: "SELECT global_director_share_status", columns: []string{"global_director_share_status"}, values: []driver.Value{int64(0)}},
				{contains: "SELECT up.id", columns: []string{"id", "earn", "price"}, values: []driver.Value{int64(20), "0", int64(1000)}},
				{contains: "UPDATE user_packages SET earn"},
			}
			tail := append([]commissionSQLStep{}, f.steps[i+1:]...)
			f.steps = append(append(f.steps[:i+1], extra...), tail...)
			break
		}
	}
	// Activation's earlier setup belongs to automatic_purchase and is already
	// covered there; retain only the activation portion for this regression.
	for len(f.steps) > 0 && !strings.Contains(f.steps[0].contains, "SELECT id, virtual_id, node") {
		f.steps = f.steps[1:]
	}
	f.steps = f.steps[:len(f.steps)-2] // caller's cap notice and intent completion
	db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
	defer db.Close()
	tx, _ := db.Beginx()
	defer tx.Rollback()
	p := &activatingPackage{ID: 9, UserID: 42, Sale: "first", PackagePrice: 100, PackageStr: "1"}
	if err := processPackageActivation(tx, p, 1, 0, 50, activationCommissionContext{IntentID: 1}); err != nil {
		t.Fatal(err)
	}
	if len(f.steps) != 0 {
		t.Fatalf("missing activation SQL: %d", len(f.steps))
	}
}

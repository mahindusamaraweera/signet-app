package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
	"time"
)

func correction486PlanSteps() []commissionSQLStep {
	at := "2026-11-20 15:52:29"
	s := []commissionSQLStep{{contains: "JOIN earn_logs e", columns: []string{"id", "earn_log_id", "beneficiary_user_id", "commission_kind", "usd", "status", "source_user", "source_amount", "event_at", "remote_id", "signet_amount"}, rowValues: [][]driver.Value{
		{int64(123), int64(1193), int64(471), "gratitude", "200", "confirmed", "471", "200", at, "77", "14221.815109418335022772"},
		{int64(124), int64(1194), int64(377), "direct_team", "500", "confirmed", "377", "500", at, "78", "35554.537773545837556930"},
		{int64(125), int64(1195), int64(1), "direct_team", "500", "accounting_only", "1", "500", at, "", ""},
		{int64(126), int64(1196), int64(377), "leadership", "50", "confirmed", "377", "50", at, "79", "3555.453777354583755693"},
		{int64(127), int64(1197), int64(1), "leadership", "50", "accounting_only", "1", "50", at, "", ""},
		{int64(128), int64(1198), int64(378), "executive", "50", "confirmed", "378", "50", at, "80", "3555.453777354583755693"},
	}}, {contains: "COUNT(*) FROM global_share_wallets_log", columns: []string{"count"}, values: []driver.Value{int64(0)}}}
	for _, id := range []int64{1, 377, 378, 471} {
		uuids := map[int64]string{1: "company", 377: "019f2876-4760-7000-8000-000000000179", 378: "019f287f-fbc0-7000-8000-00000000017a", 471: "121210a4-45da-4681-ad22-a11ac2761427"}
		s = append(s, commissionSQLStep{contains: "SELECT id,COALESCE(uuid", columns: []string{"id", "uuid"}, values: []driver.Value{id, uuids[id]}}, commissionSQLStep{contains: "FROM wallets WHERE user_id=?", columns: []string{"id", "balance"}, values: []driver.Value{id + 100, "10000.00"}})
	}
	for _, id := range []int64{378, 471} {
		s = append(s, commissionSQLStep{contains: "FROM user_packages WHERE user_id=?", columns: []string{"id", "earn", "created_at"}, values: []driver.Value{id + 100, "1000", "2026-10-01 00:00:00"}})
	}
	return s
}
func TestCorrection486PlanRefusesAmbiguousGlobalCredits(t *testing.T) {
	s := correction486PlanSteps()
	s[1].values = []driver.Value{int64(1)}
	f := &commissionSQLFixture{steps: s[:2]}
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	tx, _ := db.Beginx()
	defer tx.Rollback()
	if _, e := correction486AccountingPlan(tx); e == nil || !strings.Contains(e.Error(), "global-share") {
		t.Fatalf("%v", e)
	}
}
func TestCorrection486ExactSubtraction(t *testing.T) {
	if got, e := correction486Subtract("550.01", "550", 2); e != nil || got != "0.01" {
		t.Fatalf("%s %v", got, e)
	}
	for _, a := range []string{"49.99", "bad", "-1"} {
		if _, e := correction486Subtract(a, "50", 18); e == nil {
			t.Fatal("invalid subtraction accepted")
		}
	}
}
func correction486Proof(now time.Time, key string) ([]byte, correction486Completion) {
	p := correction486Completion{Environment: "production", Operation: Hold486Key, Buyer: Hold486UUID, Status: "recovered", Issued: now.Unix(), Expires: now.Add(30 * time.Minute).Unix(), Rows: []correction486Recovered{{77, "14221.815109418335022772", "0x" + strings.Repeat("a", 64)}, {78, "35554.537773545837556930", "0x" + strings.Repeat("b", 64)}, {79, "3555.453777354583755693", "0x" + strings.Repeat("c", 64)}, {80, "3555.453777354583755693", "0x" + strings.Repeat("d", 64)}}}
	b, _ := json.Marshal(p)
	payload := base64.StdEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte("placement-correction-recovered-v1|" + payload))
	raw, _ := json.Marshal(map[string]string{"payload": payload, "signature": hex.EncodeToString(m.Sum(nil))})
	return raw, p
}
func TestCorrection486CompletionRejectsInvalidOrExpiredSignature(t *testing.T) {
	now := time.Now()
	key := strings.Repeat("s", 32)
	proof, _ := correction486Proof(now, key)
	if _, e := correction486ReadCompletion(proof, key, now); e != nil {
		t.Fatal(e)
	}
	if _, e := correction486ReadCompletion(proof, "bad", now); e == nil {
		t.Fatal("bad signature accepted")
	}
	if _, e := correction486ReadCompletion(proof, key, now.Add(time.Hour)); e == nil {
		t.Fatal("expired receipt accepted")
	}
}
func TestCorrection486FinalizesAccountingOnceAndRetainsBuyerHold(t *testing.T) {
	key := strings.Repeat("s", 32)
	now := time.Now()
	proof, p := correction486Proof(now, key)
	planFixture := &commissionSQLFixture{steps: correction486PlanSteps()}
	planDB := sqlx.NewDb(sql.OpenDB(planFixture), "fixture")
	tx, _ := planDB.Beginx()
	plan, e := correction486AccountingPlan(tx)
	if e != nil {
		t.Fatal(e)
	}
	tx.Rollback()
	planDB.Close()
	evidence, _ := json.Marshal(map[string]interface{}{"accounting_plan": plan})
	prefix := []commissionSQLStep{{contains: "SELECT uuid,status FROM users", columns: []string{"uuid", "status"}, values: []driver.Value{Hold486UUID, "inactive"}}, {contains: "SELECT status FROM user_packages", columns: []string{"status"}, values: []driver.Value{"deactivate"}}}
	f := &commissionSQLFixture{steps: append(prefix, commissionSQLStep{contains: "SELECT evidence,status,completion", columns: []string{"evidence", "status", "completion"}, values: []driver.Value{string(evidence), "held", nil}})}
	f.steps = append(f.steps, correction486PlanSteps()...)
	for _, w := range plan.Wallets {
		after, _ := correction486Subtract(w.Before, w.Delta, 18)
		f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE wallets SET balance=?", args: []driver.Value{after, int64(w.ID), int64(w.User)}})
	}
	for _, p := range plan.Packages {
		after, _ := correction486Subtract(p.Before, p.Delta, 18)
		f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE user_packages SET earn=?", args: []driver.Value{after, int64(p.ID), int64(p.User)}})
	}
	for i := range plan.Events {
		f.steps = append(f.steps, commissionSQLStep{contains: "INSERT INTO earn_logs", insertID: int64(2000 + i)})
	}
	var completed string
	f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE placement_correction_holds SET status='recovered'", checkArgs: func(a []driver.NamedValue) error { completed = a[0].Value.(string); return nil }})
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	r, err := Finalize486(db, proof, key, now)
	if err != nil || r["source_accounting_reversed"] != true || !f.committed || len(f.steps) != 0 {
		t.Fatalf("%v %v", r, err)
	}
	f.steps = append(append([]commissionSQLStep{}, prefix...), commissionSQLStep{contains: "SELECT evidence,status,completion", columns: []string{"evidence", "status", "completion"}, values: []driver.Value{string(evidence), "recovered", completed}})
	if _, err = Finalize486(db, proof, key, now); err != nil || len(f.steps) != 0 {
		t.Fatalf("repeat %v", err)
	}
	p.Rows[0].Hash = "0x" + strings.Repeat("e", 64) // a different signed hash must not replace a completed recovery
	b, _ := json.Marshal(p)
	pl := base64.StdEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("placement-correction-recovered-v1|" + pl))
	changed, _ := json.Marshal(map[string]string{"payload": pl, "signature": hex.EncodeToString(mac.Sum(nil))})
	f.steps = append(append([]commissionSQLStep{}, prefix...), commissionSQLStep{contains: "SELECT evidence,status,completion", columns: []string{"evidence", "status", "completion"}, values: []driver.Value{string(evidence), "recovered", completed}})
	if _, err = Finalize486(db, changed, key, now); err == nil {
		t.Fatal("changed completion accepted")
	}
}

func TestCorrection486SourceFailureRollsBackAllAccountingOffsets(t *testing.T) {
	key := strings.Repeat("s", 32)
	now := time.Now()
	proof, _ := correction486Proof(now, key)
	pf := &commissionSQLFixture{steps: correction486PlanSteps()}
	pd := sqlx.NewDb(sql.OpenDB(pf), "fixture")
	pt, _ := pd.Beginx()
	plan, e := correction486AccountingPlan(pt)
	if e != nil {
		t.Fatal(e)
	}
	pt.Rollback()
	pd.Close()
	evidence, _ := json.Marshal(map[string]interface{}{"accounting_plan": plan})
	f := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "SELECT uuid,status FROM users", columns: []string{"uuid", "status"}, values: []driver.Value{Hold486UUID, "inactive"}}, {contains: "SELECT status FROM user_packages", columns: []string{"status"}, values: []driver.Value{"deactivate"}}, {contains: "SELECT evidence,status,completion", columns: []string{"evidence", "status", "completion"}, values: []driver.Value{string(evidence), "held", nil}}}}
	f.steps = append(f.steps, correction486PlanSteps()...)
	f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE wallets SET balance=?"}, commissionSQLStep{contains: "UPDATE wallets SET balance=?", err: sql.ErrConnDone})
	d := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer d.Close()
	if _, err := Finalize486(d, proof, key, now); err == nil || f.committed || !f.rolledBack || len(f.steps) != 0 {
		t.Fatal("partial accounting correction committed")
	}
}

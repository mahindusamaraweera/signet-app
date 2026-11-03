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

func recoveryPrefix(saved string) []commissionSQLStep {
	rows := [][]driver.Value{}
	if saved != "" {
		rows = append(rows, []driver.Value{saved})
	}
	return []commissionSQLStep{
		{contains: "SELECT id FROM users WHERE id=470 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(470)}},
		{contains: "FROM package_purchase_intents WHERE id=12 FOR UPDATE", columns: []string{"user_id", "user_package_id", "package_price", "status", "purchase_kind"}, values: []driver.Value{int64(470), int64(505), "1000.000000000000000000", "completed", "first"}},
		{contains: "SELECT current_user_id FROM super_parent_logs", columns: []string{"current_user_id"}, values: []driver.Value{int64(132)}},
		{contains: "SELECT evidence FROM gratitude_recoveries", columns: []string{"evidence"}, rowValues: rows},
	}
}
func recoveryEligible() []commissionSQLStep {
	return []commissionSQLStep{
		{contains: "COUNT(*) FROM blockchain_commission_outbox", columns: []string{"count"}, values: []driver.Value{int64(0)}},
		{contains: "COUNT(*) FROM blockchain_company_settlements", columns: []string{"count"}, values: []driver.Value{int64(1)}},
		{contains: "SELECT id FROM users WHERE id=132 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(132)}},
		{contains: "SELECT balance FROM wallets", columns: []string{"balance"}, values: []driver.Value{float64(50)}},
		{contains: "SUM(p.price*4)", columns: []string{"cap"}, values: []driver.Value{float64(4000)}},
		{contains: "SELECT uuid FROM users WHERE id=470", columns: []string{"uuid"}, values: []driver.Value{"01a139c5-c200-7000-8000-0000000001cc"}},
		{contains: "SELECT uuid FROM users WHERE id=132", columns: []string{"uuid"}, values: []driver.Value{"01a13b42-28d0-7000-8000-0000000001cd"}},
	}
}
func TestRecovery470DryRunAndExistingCommissionRefusal(t *testing.T) {
	for _, duplicate := range []bool{false, true} {
		f := &commissionSQLFixture{steps: recoveryPrefix("")}
		if duplicate {
			f.steps = append(f.steps, commissionSQLStep{contains: "COUNT(*) FROM blockchain_commission_outbox", columns: []string{"count"}, values: []driver.Value{int64(1)}})
		} else {
			f.steps = append(f.steps, recoveryEligible()...)
		}
		db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
		result, err := RecoverGratitude470(db, false, "", strings.Repeat("s", 32), time.Now())
		db.Close()
		if duplicate {
			if err == nil {
				t.Fatal("existing commission accepted")
			}
		} else if err != nil || result["status"] != "dry_run" {
			t.Fatalf("%v %v", result, err)
		}
		if f.committed || !f.rolledBack || len(f.steps) != 0 {
			t.Fatal("dry run mutated or missed evidence")
		}
	}
}
func TestRecovery470CreatesOneReservationAndExportsSameOnRetry(t *testing.T) {
	f := &commissionSQLFixture{steps: append(recoveryPrefix(""), recoveryEligible()...)}
	f.steps = append(f.steps, []commissionSQLStep{
		{contains: "SELECT id FROM wallets", columns: []string{"id"}, values: []driver.Value{int64(8)}},
		{contains: "UPDATE wallets", args: []driver.Value{float64(200), int64(132)}},
		{contains: "INSERT INTO earn_logs", insertID: 100, args: []driver.Value{"132", "200", "Approved missing gratitude recovery"}},
		{contains: "INSERT INTO blockchain_commission_outbox", args: []driver.Value{int64(12), int64(100), int64(132), "200", "Approved missing gratitude recovery", "gratitude"}},
		{contains: "SELECT global_director_share_status", columns: []string{"global_director_share_status"}, values: []driver.Value{int64(0)}},
		{contains: "SELECT up.id", columns: []string{"id", "earn", "price"}, values: []driver.Value{int64(20), "50", int64(1000)}},
		{contains: "UPDATE user_packages SET earn"},
		{contains: "SELECT id FROM blockchain_commission_outbox", columns: []string{"id"}, values: []driver.Value{int64(58)}},
		{contains: "UPDATE blockchain_commission_outbox SET status='recovery_pending'"},
		{contains: "INSERT INTO gratitude_recoveries"},
		{contains: "SELECT COUNT(*) FROM gratitude_recoveries r JOIN", columns: []string{"count"}, values: []driver.Value{int64(1)}},
	}...)
	db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
	defer db.Close()
	key := strings.Repeat("s", 32)
	result, err := RecoverGratitude470(db, true, "Approved missing commission for purchase 505", key, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !f.committed || len(f.steps) != 0 {
		t.Fatal("reservation not committed")
	}
	raw, _ := base64.StdEncoding.DecodeString(result["payload"].(string))
	var body struct {
		Evidence json.RawMessage `json:"evidence"`
	}
	json.Unmarshal(raw, &body)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("gratitude-recovery-v1|" + result["payload"].(string)))
	if result["signature"] != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("signature mismatch")
	}
	f.steps = append(recoveryPrefix(string(body.Evidence)), commissionSQLStep{contains: "SELECT COUNT(*) FROM gratitude_recoveries r JOIN", columns: []string{"count"}, values: []driver.Value{int64(1)}})
	if _, err = RecoverGratitude470(db, true, "different reason ignored", key, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(f.steps) != 0 {
		t.Fatal("retry did not read existing reservation")
	}
}

func TestRecovery470CannotBypassIncomeCap(t *testing.T) {
	f := &commissionSQLFixture{steps: recoveryPrefix("")}
	eligible := recoveryEligible()
	eligible[3].values = []driver.Value{float64(3900)}
	f.steps = append(f.steps, eligible[:5]...)
	db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
	defer db.Close()
	if _, err := RecoverGratitude470(db, true, "Approved recovery", strings.Repeat("s", 32), time.Now()); err == nil || !strings.Contains(err.Error(), "income limit") {
		t.Fatalf("cap not enforced: %v", err)
	}
	if f.committed || !f.rolledBack || len(f.steps) != 0 {
		t.Fatal("cap denial changed accounting")
	}
}
func TestRecovery470AcknowledgementIsIdempotentAndRejectsChangedHash(t *testing.T) {
	now := time.Now()
	key := strings.Repeat("s", 32)
	hash := "0x" + strings.Repeat("f", 64)
	body, _ := json.Marshal(map[string]interface{}{"operation": GratitudeRecoveryKey, "outbox_id": 58, "status": "completed", "tx_hash": hash, "issued_at": now.Unix(), "expires_at": now.Add(30 * time.Minute).Unix()})
	payload := base64.StdEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("gratitude-recovery-completed-v1|" + payload))
	proof, _ := json.Marshal(map[string]string{"payload": payload, "signature": hex.EncodeToString(mac.Sum(nil))})
	for _, status := range []string{"recovery_pending", "confirmed", "conflicting"} {
		var stored driver.Value
		state := status
		if status == "confirmed" {
			stored = hash
		}
		if status == "conflicting" {
			state = "confirmed"
			stored = "0x" + strings.Repeat("e", 64)
		}
		f := &commissionSQLFixture{steps: []commissionSQLStep{
			{contains: "SELECT evidence FROM gratitude_recoveries", columns: []string{"evidence"}, values: []driver.Value{"{}"}},
			{contains: "SELECT status,transfer_tx_hash", columns: []string{"status", "transfer_tx_hash"}, values: []driver.Value{state, stored}},
		}}
		if status == "recovery_pending" {
			f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE blockchain_commission_outbox SET status='confirmed'", args: []driver.Value{hash, GratitudeRecoveryKey, int64(58)}})
		}
		db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
		err := ConfirmGratitude470(db, proof, key, now)
		db.Close()
		if (err != nil) != (status == "conflicting") {
			t.Fatalf("state %s: %v", status, err)
		}
		if len(f.steps) != 0 {
			t.Fatal("unconsumed acknowledgement")
		}
	}
	f := &commissionSQLFixture{}
	db := sqlx.NewDb(sql.OpenDB(f), "commission-fixture")
	defer db.Close()
	if ConfirmGratitude470(db, proof, "wrong", now) == nil {
		t.Fatal("invalid signature accepted")
	}
	if ConfirmGratitude470(db, proof, key, now.Add(time.Hour)) == nil {
		t.Fatal("expired proof accepted")
	}
}

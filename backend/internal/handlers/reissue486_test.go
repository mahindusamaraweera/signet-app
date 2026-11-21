package handlers

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
	"time"
)

func reissuePlacementSteps(active bool) []commissionSQLStep {
	status, pkg := "inactive", "deactivate"
	if active {
		status, pkg = "active", "active"
	}
	return []commissionSQLStep{
		{contains: "SELECT uuid,status,referred_by", columns: []string{"uuid", "status", "referred_by", "leader_code", "executive_code"}, values: []driver.Value{Hold486UUID, status, int64(471), "377", "378"}},
		{contains: "SELECT status,sale,package", columns: []string{"status", "sale", "package"}, values: []driver.Value{pkg, "first", "3"}},
		{contains: "SELECT id,virtual_id,parent_id,node", columns: []string{"id", "virtual_id", "parent_id", "node"}, values: []driver.Value{int64(674), int64(471), int64(471), "active"}},
		{contains: "COUNT(*) FROM package_purchase_intents", columns: []string{"count"}, values: []driver.Value{int64(1)}},
	}
}
func reissuePrepareSteps() []commissionSQLStep {
	s := reissuePlacementSteps(false)
	s = append(s, commissionSQLStep{contains: "SELECT status FROM placement_correction_holds", columns: []string{"status"}, values: []driver.Value{"recovered"}}, commissionSQLStep{contains: "SELECT evidence,status FROM placement_commission_reissues", columns: []string{"evidence", "status"}, rowValues: [][]driver.Value{}}, commissionSQLStep{contains: "SELECT id,current_user_id,package_id,gratitude_user,user_package", columns: []string{"id", "current_user_id", "package_id", "gratitude_user", "user_package", "created_at", "updated_at"}, values: []driver.Value{int64(87), int64(471), int64(3), int64(486), int64(521), "2026-11-20 15:38:55", "2026-11-20 15:38:55"}})
	for _, id := range []int64{1, 377, 378, 471} {
		s = append(s, commissionSQLStep{contains: "SELECT id FROM users", columns: []string{"id"}, values: []driver.Value{id}}, commissionSQLStep{contains: "CAST(balance AS CHAR)", columns: []string{"balance"}, values: []driver.Value{"1000"}})
	}
	for _, line := range reissue486Lines {
		s = append(s, commissionSQLStep{contains: "SELECT uuid,status FROM users", columns: []string{"uuid", "status"}, values: []driver.Value{line.UUID, "active"}}, commissionSQLStep{contains: "SELECT balance FROM wallets", columns: []string{"balance"}, values: []driver.Value{float64(1000)}}, commissionSQLStep{contains: "SUM(p.price*4)", columns: []string{"cap"}, values: []driver.Value{float64(10000)}})
	}
	return s
}
func TestReissue486PreviewAndPrepareAreSeparateFromPaymentAndActivation(t *testing.T) {
	for _, execute := range []bool{false, true} {
		f := &commissionSQLFixture{steps: reissuePrepareSteps()}
		if execute {
			for i, line := range reissue486Lines {
				amount := float64(50)
				if i == 0 {
					amount = 500
				}
				f.steps = append(f.steps,
					commissionSQLStep{contains: "SELECT id FROM wallets", columns: []string{"id"}, values: []driver.Value{int64(line.User)}},
					commissionSQLStep{contains: "UPDATE wallets SET balance = balance +", args: []driver.Value{amount, int64(line.User)}},
					commissionSQLStep{contains: "INSERT INTO earn_logs", insertID: int64(1205 + i)},
					commissionSQLStep{contains: "SELECT global_director_share_status", columns: []string{"global_director_share_status"}, values: []driver.Value{int64(0)}},
					commissionSQLStep{contains: "SELECT up.id", columns: []string{"id", "earn", "price"}, values: []driver.Value{int64(500 + i), "1000", int64(1000)}},
					commissionSQLStep{contains: "UPDATE user_packages SET earn"},
				)
			}
			f.steps = append(f.steps, commissionSQLStep{contains: "DELETE FROM super_parent_logs WHERE id=87"}, commissionSQLStep{contains: "UPDATE user_packages SET ref_id=471"}, commissionSQLStep{contains: "INSERT INTO placement_commission_reissues"})
		}
		db := sqlx.NewDb(sql.OpenDB(f), "fixture")
		got, err := PrepareReissue486(db, execute, "Approved corrected commission plan", strings.Repeat("s", 32), time.Now())
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		if execute {
			if got["payload"] == nil || !f.committed {
				t.Fatal("missing prepared proof")
			}
		} else {
			if got["status"] != "dry_run" || f.committed {
				t.Fatal("preview mutated")
			}
		}
		if len(f.steps) != 0 {
			t.Fatalf("missing %d required queries", len(f.steps))
		}
	}
}
func TestReissue486RejectsChangedPlacementAndCaps(t *testing.T) {
	for _, change := range []string{"placement", "cap"} {
		s := reissuePrepareSteps()
		if change == "placement" {
			s[2].values[1] = int64(377)
			s = s[:3]
		} else {
			s[len(s)-1].values = []driver.Value{float64(1000)}
		}
		f := &commissionSQLFixture{steps: s}
		db := sqlx.NewDb(sql.OpenDB(f), "fixture")
		if _, err := PrepareReissue486(db, true, "Approved corrected commission plan", strings.Repeat("s", 32), time.Now()); err == nil {
			t.Fatal("unsafe plan accepted")
		}
		db.Close()
		if f.committed || !f.rolledBack {
			t.Fatal("failed preview committed")
		}
	}
}
func TestReissue486CompletionActivatesOnlyOnceAndRejectsChangedReceipts(t *testing.T) {
	key := strings.Repeat("s", 32)
	now := time.Now()
	rows := []reissue486Receipt{}
	for i, l := range reissue486Lines {
		rows = append(rows, reissue486Receipt{l.Kind, l.User, l.USD, "100", "0x" + strings.Repeat(string(rune('a'+i)), 64)})
	}
	signed, err := reissue486Sign("placement-486-reissue-completed", map[string]interface{}{"operation": Reissue486Key, "buyer_uuid": Hold486UUID, "commissions": rows}, key, now)
	if err != nil {
		t.Fatal(err)
	}
	proof, _ := json.Marshal(signed)
	for _, state := range []string{"prepared", "completed", "mismatch", "write_failure"} {
		saved, _ := json.Marshal(rows)
		if state == "mismatch" {
			saved = []byte(`[]`)
		}
		status := "prepared"
		active := false
		if state == "completed" || state == "mismatch" {
			status = "completed"
			active = true
		}
		f := &commissionSQLFixture{steps: []commissionSQLStep{{contains: "SELECT id FROM users WHERE id=486 FOR UPDATE", columns: []string{"id"}, values: []driver.Value{int64(486)}}, {contains: "SELECT status,completion", columns: []string{"status", "completion"}, values: []driver.Value{status, string(saved)}}}}
		if state != "mismatch" {
			f.steps = append(f.steps, reissuePlacementSteps(active)...)
			f.steps = append(f.steps, commissionSQLStep{contains: "COUNT(*) FROM super_parent_logs", columns: []string{"count"}, values: []driver.Value{int64(0)}})
		}
		if state == "prepared" || state == "write_failure" {
			f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE users SET status='active'"})
			if state == "write_failure" {
				f.steps[len(f.steps)-1].err = errors.New("storage failed")
			} else {
				f.steps = append(f.steps, commissionSQLStep{contains: "UPDATE user_packages SET status='active'"}, commissionSQLStep{contains: "UPDATE placement_commission_reissues SET status='completed'"})
			}
		}
		db := sqlx.NewDb(sql.OpenDB(f), "fixture")
		got, e := CompleteReissue486(db, proof, key, now)
		db.Close()
		if state == "mismatch" || state == "write_failure" {
			if e == nil || f.committed {
				t.Fatal("failed completion committed")
			}
		} else if e != nil || got["payload"] == nil || !f.committed {
			t.Fatalf("%s: %v", state, e)
		}
		if len(f.steps) != 0 {
			t.Fatalf("%s left %d queries", state, len(f.steps))
		}
	}
	var result map[string]interface{}
	if err = reissue486Read(proof, "placement-486-reissue-completed", key, now.Add(time.Hour), &result); err == nil {
		t.Fatal("expired proof accepted")
	}
}

func TestReissue486PreparedRetryOnlyReexportsProof(t *testing.T) {
	key := strings.Repeat("s", 32)
	now := time.Now()
	evidence := map[string]interface{}{"operation": Reissue486Key, "buyer_uuid": Hold486UUID, "source_user": 486, "source_package_id": 521, "source_intent_id": 29, "custody_purchase_id": 59, "lines": reissue486Lines, "gratitude_usd": "0", "retained_usd": "200", "earn_log_ids": []int64{1205, 1206, 1207}, "reason": "Approved corrected commission plan"}
	b, _ := json.Marshal(evidence)
	f := &commissionSQLFixture{steps: reissuePlacementSteps(false)}
	f.steps = append(f.steps,
		commissionSQLStep{contains: "SELECT status FROM placement_correction_holds", columns: []string{"status"}, values: []driver.Value{"recovered"}},
		commissionSQLStep{contains: "SELECT evidence,status FROM placement_commission_reissues", columns: []string{"evidence", "status"}, values: []driver.Value{string(b), "prepared"}},
		commissionSQLStep{contains: "COUNT(*) FROM super_parent_logs", columns: []string{"count"}, values: []driver.Value{int64(0)}},
	)
	db := sqlx.NewDb(sql.OpenDB(f), "fixture")
	defer db.Close()
	proof, err := PrepareReissue486(db, true, "", key, now)
	if err != nil || !f.committed || len(f.steps) != 0 {
		t.Fatalf("retry failed: %v", err)
	}
	raw, _ := json.Marshal(proof)
	var actual map[string]interface{}
	if err = reissue486Read(raw, "placement-486-reissue", key, now, &actual); err != nil || actual["operation"] != Reissue486Key {
		t.Fatalf("proof changed: %v", err)
	}
}

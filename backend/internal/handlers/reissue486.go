package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	"reflect"
	"regexp"
	"signet-backend/internal/wallet"
	"strings"
	"time"
)

const Reissue486Key = "placement-486-reissue-v1"

type reissue486Line struct {
	Kind string `json:"kind"`
	User uint64 `json:"user_id"`
	UUID string `json:"uuid"`
	USD  string `json:"usd"`
}

var reissue486Lines = []reissue486Line{
	{"direct_team", 471, "121210a4-45da-4681-ad22-a11ac2761427", "500"},
	{"leadership", 377, "019f2876-4760-7000-8000-000000000179", "50"},
	{"executive", 378, "019f287f-fbc0-7000-8000-00000000017a", "50"},
}

func reissue486Sign(domain string, value interface{}, key string, now time.Time) (map[string]interface{}, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("integration credential required")
	}
	raw, err := json.Marshal(map[string]interface{}{"environment": "production", "issued_at": now.Unix(), "expires_at": now.Add(30 * time.Minute).Unix(), "evidence": value})
	if err != nil {
		return nil, err
	}
	payload := base64.StdEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(domain + "|" + payload))
	return map[string]interface{}{"payload": payload, "signature": hex.EncodeToString(mac.Sum(nil))}, nil
}
func reissue486Read(raw []byte, domain, key string, now time.Time, out interface{}) error {
	var e struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if len(key) < 32 || len(raw) > 1048576 || json.Unmarshal(raw, &e) != nil {
		return fmt.Errorf("invalid proof")
	}
	sig, err := hex.DecodeString(e.Signature)
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(domain + "|" + e.Payload))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return fmt.Errorf("invalid proof signature")
	}
	b, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return err
	}
	var p struct {
		Environment string          `json:"environment"`
		Issued      int64           `json:"issued_at"`
		Expires     int64           `json:"expires_at"`
		Evidence    json.RawMessage `json:"evidence"`
	}
	if json.Unmarshal(b, &p) != nil || p.Environment != "production" || p.Issued > now.Unix()+60 || p.Expires <= now.Unix() || p.Expires-p.Issued != 1800 {
		return fmt.Errorf("expired or invalid proof")
	}
	return json.Unmarshal(p.Evidence, out)
}

// Checks the exact operator-approved corrected placement; never runs activation.
func reissue486Placement(tx *sqlx.Tx, active bool) error {
	var buyer struct {
		UUID      string `db:"uuid"`
		Status    string `db:"status"`
		Ref       uint64 `db:"referred_by"`
		Leader    string `db:"leader_code"`
		Executive string `db:"executive_code"`
	}
	if err := tx.Get(&buyer, "SELECT uuid,status,referred_by,leader_code,executive_code FROM users WHERE id=486 FOR UPDATE"); err != nil {
		return err
	}
	status := "inactive"
	if active {
		status = "active"
	}
	if buyer.UUID != Hold486UUID || buyer.Status != status || buyer.Ref != 471 || buyer.Leader != "377" || buyer.Executive != "378" {
		return fmt.Errorf("approved buyer identity, status or leadership changed")
	}
	var pkg struct {
		Status  string `db:"status"`
		Sale    string `db:"sale"`
		Package string `db:"package"`
	}
	if err := tx.Get(&pkg, "SELECT status,sale,package FROM user_packages WHERE id=521 AND user_id=486 FOR UPDATE"); err != nil {
		return err
	}
	if pkg.Sale != "first" || pkg.Package != "3" || (active && pkg.Status != "active") || (!active && pkg.Status != "deactivate" && pkg.Status != "deactive") {
		return fmt.Errorf("original buyer package changed")
	}
	var nodes []struct {
		ID      uint64 `db:"id"`
		Virtual uint64 `db:"virtual_id"`
		Parent  uint64 `db:"parent_id"`
		Node    string `db:"node"`
	}
	if err := tx.Select(&nodes, "SELECT id,virtual_id,parent_id,node FROM user_parents WHERE user_id=486 ORDER BY id FOR UPDATE"); err != nil {
		return err
	}
	if len(nodes) != 1 || nodes[0].ID != 674 || nodes[0].Virtual != 471 || nodes[0].Parent != 471 || nodes[0].Node != "active" {
		return fmt.Errorf("corrected placement must be sole row 674, virtual/parent 471, active")
	}
	var n int
	if err := tx.Get(&n, "SELECT COUNT(*) FROM package_purchase_intents WHERE id=29 AND user_id=486 AND user_package_id=521 AND status='completed' AND remote_order_id=59 AND package_price=1000 AND commission_user_id=377 AND commission_percent=50"); err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("original completed purchase changed; do not reset it")
	}
	return nil
}

func PrepareReissue486(db *sqlx.DB, execute bool, reason, key string, now time.Time) (map[string]interface{}, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("integration credential required")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = reissue486Placement(tx, false); err != nil {
		return nil, err
	}
	var recovered string
	if err = tx.Get(&recovered, "SELECT status FROM placement_correction_holds WHERE operation_key=? FOR UPDATE", Hold486Key); err != nil {
		return nil, err
	}
	if recovered != "recovered" {
		return nil, fmt.Errorf("source recovery must be complete")
	}
	var saved []struct {
		Evidence string `db:"evidence"`
		Status   string `db:"status"`
	}
	if err = tx.Select(&saved, "SELECT evidence,status FROM placement_commission_reissues WHERE operation_key=? FOR UPDATE", Reissue486Key); err != nil {
		return nil, err
	}
	if len(saved) > 0 {
		if saved[0].Status != "prepared" {
			return nil, fmt.Errorf("reissue already completed; use completion proof mode")
		}
		var e map[string]interface{}
		if err = json.Unmarshal([]byte(saved[0].Evidence), &e); err != nil {
			return nil, err
		}
		var n int
		if err = tx.Get(&n, "SELECT COUNT(*) FROM super_parent_logs WHERE gratitude_user=486 OR user_package=521"); err != nil {
			return nil, err
		}
		if n != 0 {
			return nil, fmt.Errorf("obsolete gratitude entitlement reappeared")
		}
		if !execute {
			return map[string]interface{}{"status": "prepared", "evidence": e, "funds_sent": false}, nil
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return reissue486Sign("placement-486-reissue", e, key, now)
	}
	var gratitude []struct {
		ID        uint64 `db:"id"`
		Recipient uint64 `db:"current_user_id"`
		Package   uint64 `db:"package_id"`
		Buyer     uint64 `db:"gratitude_user"`
		Purchase  uint64 `db:"user_package"`
		Created   string `db:"created_at"`
		Updated   string `db:"updated_at"`
	}
	if err = tx.Select(&gratitude, "SELECT id,current_user_id,package_id,gratitude_user,user_package,CAST(created_at AS CHAR) AS created_at,CAST(updated_at AS CHAR) AS updated_at FROM super_parent_logs WHERE gratitude_user=486 OR user_package=521 ORDER BY id FOR UPDATE"); err != nil {
		return nil, err
	}
	if len(gratitude) != 1 || gratitude[0].ID != 87 || gratitude[0].Recipient != 471 || gratitude[0].Package != 3 || gratitude[0].Buyer != 486 || gratitude[0].Purchase != 521 {
		return nil, fmt.Errorf("expected obsolete gratitude row 87 changed; review before archiving")
	}
	// Lock all recipients in a deterministic order, checking caps without silently rerouting.
	for _, id := range []uint64{1, 377, 378, 471} {
		var actual uint64
		if err = tx.Get(&actual, "SELECT id FROM users WHERE id=? FOR UPDATE", id); err != nil {
			return nil, err
		}
		var balance string
		if err = tx.Get(&balance, "SELECT CAST(balance AS CHAR) FROM wallets WHERE user_id=? FOR UPDATE", id); err != nil {
			return nil, err
		}
	}
	for _, l := range reissue486Lines {
		var u struct {
			UUID   string `db:"uuid"`
			Status string `db:"status"`
		}
		if err = tx.Get(&u, "SELECT uuid,status FROM users WHERE id=?", l.User); err != nil {
			return nil, err
		}
		if u.UUID != l.UUID || u.Status != "active" {
			return nil, fmt.Errorf("recipient %d identity/status changed", l.User)
		}
		var balance float64
		var cap float64
		if err = tx.Get(&balance, "SELECT balance FROM wallets WHERE user_id=?", l.User); err != nil {
			return nil, err
		}
		if err = tx.Get(&cap, `SELECT COALESCE(SUM(p.price*4),0) - (SELECT COALESCE(SUM(commission_usd),0) FROM blockchain_topup_commission_plans WHERE beneficiary_user_id=? AND status='reserved') FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=? AND up.status='active'`, l.User, l.User); err != nil {
			return nil, err
		}
		amount := float64(50)
		if l.User == 471 {
			amount = 500
		}
		if !wallet.CommissionFitsCap(balance, amount, cap) {
			return nil, fmt.Errorf("recipient %d income cap requires review; no diversion or payment", l.User)
		}
	}
	e := map[string]interface{}{"operation": Reissue486Key, "buyer_uuid": Hold486UUID, "source_user": 486, "source_package_id": 521, "source_intent_id": 29, "custody_purchase_id": 59, "lines": reissue486Lines, "gratitude_usd": "0", "retained_usd": "200", "archived_gratitude": gratitude, "reason": strings.TrimSpace(reason)}
	if !execute {
		return map[string]interface{}{"status": "dry_run", "evidence": e, "funds_sent": false, "buyer_still_held": true}, nil
	}
	if len(strings.TrimSpace(reason)) < 10 || len(reason) > 1000 {
		return nil, fmt.Errorf("operator reason of 10–1000 characters required")
	}
	// Use normal source wallet/global-share/package accounting, without creating a normal payment outbox.
	var logs []int64
	for _, l := range reissue486Lines {
		amount := float64(50)
		if l.User == 471 {
			amount = 500
		}
		record := func(tx *sqlx.Tx, id int64, user uint64, value, desc string) error {
			logs = append(logs, id)
			return nil
		}
		if err = creditWalletTx(tx, l.User, amount, "Placement 486 corrected "+l.Kind+" commission", record); err != nil {
			return nil, err
		}
	}
	e["earn_log_ids"] = logs
	result, err := tx.Exec("DELETE FROM super_parent_logs WHERE id=87 AND current_user_id=471 AND gratitude_user=486 AND user_package=521")
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return nil, fmt.Errorf("gratitude archive removal failed")
	}
	// ref_id is the current package referral; historical purchase recipient stays 377.
	if _, err = tx.Exec("UPDATE user_packages SET ref_id=471,updated_at=NOW() WHERE id=521 AND user_id=486"); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(e)
	if _, err = tx.Exec("INSERT INTO placement_commission_reissues (operation_key,evidence,status) VALUES (?,?,'prepared')", Reissue486Key, string(raw)); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return reissue486Sign("placement-486-reissue", e, key, now)
}

type reissue486Receipt struct {
	Kind   string `json:"kind"`
	User   uint64 `json:"user_id"`
	USD    string `json:"usd"`
	Amount string `json:"amount"`
	Hash   string `json:"tx_hash"`
}

func validateReissue486Receipts(rows []reissue486Receipt) error {
	if len(rows) != 3 {
		return fmt.Errorf("three completed payments required")
	}
	seen := map[string]bool{}
	for i, r := range rows {
		e := reissue486Lines[i]
		if r.Kind != e.Kind || r.User != e.User || r.USD != e.USD || !regexp.MustCompile(`^0x[a-f0-9]{64}$`).MatchString(r.Hash) || seen[r.Hash] {
			return fmt.Errorf("replacement payment evidence changed")
		}
		v, ok := newRat(r.Amount)
		if !ok || v == "0" || strings.HasPrefix(v, "-") {
			return fmt.Errorf("invalid replacement amount")
		}
		seen[r.Hash] = true
	}
	return nil
}

// Completion only activates existing records; never runs purchase processing or pays again.
func CompleteReissue486(db *sqlx.DB, raw []byte, key string, now time.Time) (map[string]interface{}, error) {
	var p struct {
		Operation string              `json:"operation"`
		Buyer     string              `json:"buyer_uuid"`
		Rows      []reissue486Receipt `json:"commissions"`
	}
	if err := reissue486Read(raw, "placement-486-reissue-completed", key, now, &p); err != nil {
		return nil, err
	}
	if p.Operation != Reissue486Key || p.Buyer != Hold486UUID {
		return nil, fmt.Errorf("wrong replacement operation")
	}
	if err := validateReissue486Receipts(p.Rows); err != nil {
		return nil, err
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	var completion sql.NullString
	// Buyer first matches every source correction command lock order.
	var id uint64
	if err = tx.Get(&id, "SELECT id FROM users WHERE id=486 FOR UPDATE"); err != nil {
		return nil, err
	}
	if err = tx.QueryRowx("SELECT status,completion FROM placement_commission_reissues WHERE operation_key=? FOR UPDATE", Reissue486Key).Scan(&status, &completion); err != nil {
		return nil, err
	}
	if status == "completed" {
		var saved []reissue486Receipt
		if json.Unmarshal([]byte(completion.String), &saved) != nil || !reflect.DeepEqual(saved, p.Rows) {
			return nil, fmt.Errorf("completed replacement receipts changed")
		}
	}
	if status != "prepared" && status != "completed" {
		return nil, fmt.Errorf("replacement source reservation missing")
	}
	if err = reissue486Placement(tx, status == "completed"); err != nil {
		return nil, err
	}
	var n int
	if err = tx.Get(&n, "SELECT COUNT(*) FROM super_parent_logs WHERE gratitude_user=486 OR user_package=521"); err != nil {
		return nil, err
	}
	if n != 0 {
		return nil, fmt.Errorf("gratitude entitlement must remain absent")
	}
	if status == "prepared" {
		b, _ := json.Marshal(p.Rows)
		for _, q := range []struct {
			SQL  string
			Args []interface{}
		}{
			{"UPDATE users SET status='active',updated_at=NOW() WHERE id=486 AND status='inactive'", nil},
			{"UPDATE user_packages SET status='active',updated_at=NOW() WHERE id=521 AND user_id=486 AND status IN ('deactivate','deactive')", nil},
			{"UPDATE placement_commission_reissues SET status='completed',completion=? WHERE operation_key=? AND status='prepared'", []interface{}{string(b), Reissue486Key}},
		} {
			r, e := tx.Exec(q.SQL, q.Args...)
			if e != nil {
				return nil, e
			}
			count, e := r.RowsAffected()
			if e != nil || count != 1 {
				return nil, fmt.Errorf("reactivation not persisted")
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return reissue486Sign("placement-486-release", map[string]interface{}{"operation": Reissue486Key, "buyer_uuid": Hold486UUID, "source_active": true, "commissions": p.Rows}, key, now)
}

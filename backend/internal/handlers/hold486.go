package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jmoiron/sqlx"
	"strings"
	"time"
)

const Hold486Key = "placement-correction-486-29-v1"
const Hold486UUID = "1dde762b-4d54-4529-86ff-28b8cee471b4"

// Hold only. Never changes the completed intent, original commissions, placement,
// earnings or wallets. Exported proof authorizes only the matching custody hold.
func Hold486(db *sqlx.DB, execute bool, reason, key string, now time.Time) (map[string]interface{}, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("integration credential required")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var u struct {
		UUID   string `db:"uuid"`
		Status string `db:"status"`
	}
	if err = tx.Get(&u, "SELECT uuid,status FROM users WHERE id=486 FOR UPDATE"); err != nil {
		return nil, err
	}
	if u.UUID != Hold486UUID {
		return nil, fmt.Errorf("reviewed customer UUID changed")
	}
	var p struct {
		User    uint64 `db:"user_id"`
		Package string `db:"package"`
		Status  string `db:"status"`
		Sale    string `db:"sale"`
		Earn    string `db:"earn"`
	}
	if err = tx.Get(&p, "SELECT user_id,package,status,sale,earn FROM user_packages WHERE id=521 FOR UPDATE"); err != nil {
		return nil, err
	}
	earned, earnedOK := newRat(p.Earn)
	if p.User != 486 || p.Package != "3" || p.Sale != "first" || !earnedOK || earned != "0" {
		return nil, fmt.Errorf("reviewed package changed or has earnings")
	}
	var valid int
	if err = tx.Get(&valid, `SELECT COUNT(*) FROM package_purchase_intents WHERE id=29 AND user_id=486 AND user_package_id=521 AND package_price=1000 AND status='completed' AND purchase_kind='first' FOR UPDATE`); err != nil {
		return nil, err
	}
	if valid != 1 {
		return nil, fmt.Errorf("completed purchase 29 required")
	}
	var rows []struct {
		ID     uint64  `db:"id"`
		User   uint64  `db:"beneficiary_user_id"`
		Kind   string  `db:"commission_kind"`
		USD    string  `db:"commission_usd"`
		Status string  `db:"status"`
		Hash   *string `db:"transfer_tx_hash"`
	}
	if err = tx.Select(&rows, `SELECT id,beneficiary_user_id,commission_kind,commission_usd,status,transfer_tx_hash FROM blockchain_commission_outbox WHERE purchase_intent_id=29 ORDER BY id FOR UPDATE`); err != nil {
		return nil, err
	}
	expected := []struct {
		ID, User          uint64
		Kind, USD, Status string
	}{{123, 471, "gratitude", "200", "confirmed"}, {124, 377, "direct_team", "500", "confirmed"}, {125, 1, "direct_team", "500", "accounting_only"}, {126, 377, "leadership", "50", "confirmed"}, {127, 1, "leadership", "50", "accounting_only"}, {128, 378, "executive", "50", "confirmed"}}
	if len(rows) != len(expected) {
		return nil, fmt.Errorf("original commission manifest changed")
	}
	for i, r := range rows {
		e := expected[i]
		a, ok := newRat(r.USD)
		if !ok || r.ID != e.ID || r.User != e.User || r.Kind != e.Kind || a != e.USD || r.Status != e.Status || (e.Status == "confirmed" && (r.Hash == nil || len(*r.Hash) != 66)) {
			return nil, fmt.Errorf("original commission evidence changed")
		}
	}
	var saved []string
	if err = tx.Select(&saved, "SELECT evidence FROM placement_correction_holds WHERE operation_key=? AND status='held' FOR UPDATE", Hold486Key); err != nil {
		return nil, err
	}
	var evidence map[string]interface{}
	if len(saved) > 0 {
		if u.Status != "inactive" || p.Status != "deactive" && p.Status != "deactivate" {
			return nil, fmt.Errorf("source correction hold was removed; reconcile")
		}
		if err = json.Unmarshal([]byte(saved[0]), &evidence); err != nil {
			return nil, err
		}
	} else {
		if u.Status != "active" || p.Status != "active" {
			return nil, fmt.Errorf("expected active account and package")
		}
		plan, planErr := correction486AccountingPlan(tx)
		if planErr != nil {
			return nil, planErr
		}
		var column struct {
			Type string `db:"Type"`
		}
		// MySQL schemas in this deployment use either legacy enum spelling.
		rows, schemaErr := tx.Queryx("SHOW COLUMNS FROM user_packages LIKE 'status'")
		if schemaErr != nil {
			return nil, schemaErr
		}
		packageHold := "deactivate"
		if rows.Next() {
			values, e := rows.SliceScan()
			if e != nil {
				rows.Close()
				return nil, e
			}
			column.Type = fmt.Sprint(values[1])
			if b, ok := values[1].([]byte); ok {
				column.Type = string(b)
			}
		} else {
			rows.Close()
			return nil, fmt.Errorf("package status schema missing")
		}
		rows.Close()
		if strings.HasPrefix(column.Type, "enum(") {
			if strings.Contains(column.Type, "'deactivate'") {
				packageHold = "deactivate"
			} else if strings.Contains(column.Type, "'deactive'") {
				packageHold = "deactive"
			} else {
				return nil, fmt.Errorf("schema has no supported inactive package status")
			}
		}
		evidence = map[string]interface{}{"operation": Hold486Key, "buyer_uuid": Hold486UUID, "source_user": 486, "source_package_id": 521, "source_intent_id": 29, "custody_purchase_id": 59, "reason": strings.TrimSpace(reason), "original_user_status": u.Status, "original_package_status": p.Status, "held_at": now.UTC().Format(time.RFC3339)}
		evidence["accounting_plan"] = plan
		evidence["held_package_status"] = packageHold
		if !execute {
			return map[string]interface{}{"status": "dry_run", "evidence": evidence, "funds_sent": false}, nil
		}
		if len(strings.TrimSpace(reason)) < 10 || len(reason) > 1000 {
			return nil, fmt.Errorf("operator reason of 10–1000 characters required")
		}
		b, _ := json.Marshal(evidence)
		if _, err = tx.Exec("INSERT INTO placement_correction_holds (operation_key,user_id,evidence) VALUES (?,486,?)", Hold486Key, string(b)); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE users SET status='inactive',updated_at=NOW() WHERE id=486"); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE user_packages SET status=?,updated_at=NOW() WHERE id=521", packageHold); err != nil {
			return nil, err
		}
	}
	if !execute {
		return map[string]interface{}{"status": "held", "evidence": evidence, "funds_sent": false}, nil
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	b, _ := json.Marshal(map[string]interface{}{"environment": "production", "issued_at": now.Unix(), "expires_at": now.Add(30 * time.Minute).Unix(), "evidence": evidence})
	payload := base64.StdEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("placement-correction-hold-v1|" + payload))
	return map[string]interface{}{"payload": payload, "signature": hex.EncodeToString(mac.Sum(nil))}, nil
}

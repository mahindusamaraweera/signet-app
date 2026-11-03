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
	"math/big"
	"regexp"
	"signet-backend/internal/wallet"
	"strings"
	"time"
)

const GratitudeRecoveryKey = "gratitude-470-132-505-v1"

// Fixed operator-approved correction. This never changes a sealed manifest or
// invokes a signer. USD accounting and the recovery marker commit together.
func RecoverGratitude470(db *sqlx.DB, execute bool, reason, key string, now time.Time) (map[string]interface{}, error) {
	if len(key) < 32 {
		return nil, fmt.Errorf("integration credential required")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var buyer uint64
	if err = tx.Get(&buyer, "SELECT id FROM users WHERE id=470 FOR UPDATE"); err != nil {
		return nil, err
	}
	var intent struct {
		User    uint64 `db:"user_id"`
		Package uint64 `db:"user_package_id"`
		Price   string `db:"package_price"`
		Status  string `db:"status"`
		Kind    string `db:"purchase_kind"`
	}
	if err = tx.Get(&intent, "SELECT user_id,user_package_id,package_price,status,purchase_kind FROM package_purchase_intents WHERE id=12 FOR UPDATE"); err != nil {
		return nil, err
	}
	price, ok := newRat(intent.Price)
	if !ok || price != "1000" || intent.User != 470 || intent.Package != 505 || intent.Status != "completed" || intent.Kind != "first" {
		return nil, fmt.Errorf("reviewed purchase evidence changed")
	}
	recipient, err := firstGratitudeRecipient(tx, 470, 505, "active", 132)
	if err != nil {
		return nil, err
	}
	if recipient != 132 {
		return nil, fmt.Errorf("reviewed gratitude recipient changed")
	}
	var saved []string
	if err = tx.Select(&saved, "SELECT evidence FROM gratitude_recoveries WHERE operation_key=? FOR UPDATE", GratitudeRecoveryKey); err != nil {
		return nil, err
	}
	var evidence map[string]interface{}
	if len(saved) > 0 {
		if err = json.Unmarshal([]byte(saved[0]), &evidence); err != nil {
			return nil, err
		}
	} else {
		var existing int
		if err = tx.Get(&existing, "SELECT COUNT(*) FROM blockchain_commission_outbox WHERE purchase_intent_id=12 AND commission_kind='gratitude'"); err != nil {
			return nil, err
		}
		if existing != 0 {
			return nil, fmt.Errorf("gratitude instruction already exists; reconcile, do not pay again")
		}
		var settled int
		if err = tx.Get(&settled, "SELECT COUNT(*) FROM blockchain_company_settlements WHERE purchase_intent_id=12 AND status='confirmed'"); err != nil {
			return nil, err
		}
		if settled != 1 {
			return nil, fmt.Errorf("original company settlement must be confirmed before separate recovery")
		}
		var id uint64
		if err = tx.Get(&id, "SELECT id FROM users WHERE id=132 FOR UPDATE"); err != nil {
			return nil, err
		}
		var balance float64
		if err = tx.Get(&balance, "SELECT balance FROM wallets WHERE user_id=132 FOR UPDATE"); err != nil {
			return nil, err
		}
		var cap float64
		if err = tx.Get(&cap, `SELECT COALESCE(SUM(p.price*4),0) - (SELECT COALESCE(SUM(commission_usd),0) FROM blockchain_topup_commission_plans WHERE beneficiary_user_id=132 AND status='reserved') FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=132 AND up.status='active'`); err != nil {
			return nil, err
		}
		if !wallet.CommissionFitsCap(balance, 200, cap) {
			return nil, fmt.Errorf("recipient income limit requires review; no recovery or diversion created")
		}
		var buyerUUID, recipientUUID string
		if err = tx.Get(&buyerUUID, "SELECT uuid FROM users WHERE id=470"); err != nil {
			return nil, err
		}
		if err = tx.Get(&recipientUUID, "SELECT uuid FROM users WHERE id=132"); err != nil {
			return nil, err
		}
		if len(buyerUUID) != 36 || len(recipientUUID) != 36 || buyerUUID == recipientUUID {
			return nil, fmt.Errorf("invalid custody UUIDs")
		}
		evidence = map[string]interface{}{"operation": GratitudeRecoveryKey, "buyer_uuid": buyerUUID, "recipient_uuid": recipientUUID, "source_user": 470, "recipient_user": 132, "source_package_id": 505, "purchase_intent_id": 12, "commission_usd": "200", "reason": strings.TrimSpace(reason)}
		if !execute {
			return map[string]interface{}{"status": "dry_run", "evidence": evidence, "funds_sent": false}, nil
		}
		if len(strings.TrimSpace(reason)) < 10 || len(reason) > 1000 {
			return nil, fmt.Errorf("operator reason of 10–1000 characters required")
		}
		if err = creditWalletTx(tx, 132, 200, "Approved missing gratitude recovery", purchaseCommissionRecorder(12, "gratitude")); err != nil {
			return nil, err
		}
		var outbox uint64
		if err = tx.Get(&outbox, "SELECT id FROM blockchain_commission_outbox WHERE purchase_intent_id=12 AND beneficiary_user_id=132 AND commission_kind='gratitude' FOR UPDATE"); err != nil {
			return nil, err
		}
		if _, err = tx.Exec("UPDATE blockchain_commission_outbox SET status='recovery_pending' WHERE id=? AND status='awaiting_policy'", outbox); err != nil {
			return nil, err
		}
		evidence["outbox_id"] = outbox
		encoded, _ := json.Marshal(evidence)
		if _, err = tx.Exec("INSERT INTO gratitude_recoveries (operation_key,purchase_intent_id,outbox_id,evidence) VALUES (?,12,?,?)", GratitudeRecoveryKey, outbox, string(encoded)); err != nil {
			return nil, err
		}
	}
	var valid int
	if err = tx.Get(&valid, `SELECT COUNT(*) FROM gratitude_recoveries r JOIN blockchain_commission_outbox o ON o.id=r.outbox_id WHERE r.operation_key=? AND r.purchase_intent_id=12 AND o.purchase_intent_id=12 AND o.beneficiary_user_id=132 AND o.commission_kind='gratitude' AND o.commission_usd=200 AND o.status IN ('recovery_pending','confirmed')`, GratitudeRecoveryKey); err != nil {
		return nil, err
	}
	if valid != 1 {
		return nil, fmt.Errorf("recovery accounting evidence changed")
	}
	if !execute {
		return map[string]interface{}{"status": "prepared", "evidence": evidence, "funds_sent": false}, nil
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]interface{}{"environment": "production", "issued_at": now.Unix(), "expires_at": now.Add(30 * time.Minute).Unix(), "evidence": evidence})
	encoded := base64.StdEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("gratitude-recovery-v1|" + encoded))
	return map[string]interface{}{"payload": encoded, "signature": hex.EncodeToString(mac.Sum(nil))}, nil
}

func newRat(s string) (string, bool) {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return "", false
	}
	return r.RatString(), true
}

// Signed custody acknowledgement updates delivery status only, never balances.
func ConfirmGratitude470(db *sqlx.DB, proof []byte, key string, now time.Time) error {
	var envelope struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if len(key) < 32 || json.Unmarshal(proof, &envelope) != nil {
		return fmt.Errorf("invalid completion proof")
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("gratitude-recovery-completed-v1|" + envelope.Payload))
	sig, err := hex.DecodeString(envelope.Signature)
	if err != nil || !hmac.Equal(mac.Sum(nil), sig) {
		return fmt.Errorf("invalid completion signature")
	}
	raw, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil {
		return err
	}
	var p struct {
		Operation string `json:"operation"`
		Outbox    uint64 `json:"outbox_id"`
		Hash      string `json:"tx_hash"`
		Expires   int64  `json:"expires_at"`
		Issued    int64  `json:"issued_at"`
		Status    string `json:"status"`
	}
	if json.Unmarshal(raw, &p) != nil || p.Operation != GratitudeRecoveryKey || p.Status != "completed" || p.Outbox == 0 || p.Issued > now.Unix()+60 || p.Expires <= now.Unix() || p.Expires-p.Issued != 1800 || !regexp.MustCompile(`^0x[a-f0-9]{64}$`).MatchString(p.Hash) {
		return fmt.Errorf("invalid completion evidence")
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var evidence string
	if err = tx.Get(&evidence, "SELECT evidence FROM gratitude_recoveries WHERE operation_key=? AND outbox_id=? FOR UPDATE", GratitudeRecoveryKey, p.Outbox); err != nil {
		return err
	}
	var row struct {
		Status string         `db:"status"`
		Hash   sql.NullString `db:"transfer_tx_hash"`
	}
	if err = tx.Get(&row, "SELECT status,transfer_tx_hash FROM blockchain_commission_outbox WHERE id=? AND purchase_intent_id=12 AND beneficiary_user_id=132 AND commission_kind='gratitude' AND commission_usd=200 FOR UPDATE", p.Outbox); err != nil {
		return err
	}
	if row.Status == "confirmed" {
		if row.Hash.String != p.Hash {
			return fmt.Errorf("completion transaction changed")
		}
		return tx.Commit()
	}
	if row.Status != "recovery_pending" || row.Hash.Valid {
		return fmt.Errorf("recovery delivery state changed")
	}
	if _, err = tx.Exec("UPDATE blockchain_commission_outbox SET status='confirmed',transfer_tx_hash=?,remote_payment_id=? WHERE id=?", p.Hash, GratitudeRecoveryKey, p.Outbox); err != nil {
		return err
	}
	return tx.Commit()
}

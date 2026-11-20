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
	"reflect"
	"regexp"
	"strconv"
	"time"
)

type correction486Event struct {
	ID           uint64 `db:"id" json:"outbox_id"`
	Earn         uint64 `db:"earn_log_id" json:"earn_log_id"`
	User         uint64 `db:"beneficiary_user_id" json:"user_id"`
	Kind         string `db:"commission_kind" json:"kind"`
	USD          string `db:"usd" json:"usd"`
	Status       string `db:"status" json:"status"`
	SourceUser   string `db:"source_user" json:"source_user"`
	SourceAmount string `db:"source_amount" json:"source_amount"`
	At           string `db:"event_at" json:"event_at"`
	Remote       string `db:"remote_id" json:"remote_id"`
	Tokens       string `db:"signet_amount" json:"signet_amount"`
}
type correction486Wallet struct {
	ID     uint64 `db:"id" json:"id"`
	User   uint64 `json:"user_id"`
	Before string `db:"balance" json:"before"`
	Delta  string `json:"delta"`
}
type correction486Earn struct {
	ID      uint64 `db:"id" json:"id"`
	User    uint64 `json:"user_id"`
	Before  string `db:"earn" json:"before"`
	Created string `db:"created_at" json:"created_at"`
	Delta   string `json:"delta"`
}
type correction486Plan struct {
	Events   []correction486Event  `json:"events"`
	Wallets  []correction486Wallet `json:"wallets"`
	Packages []correction486Earn   `json:"packages"`
}

// Legacy global-share credits have no purchase FK and may be partially capped.
// Refuse ambiguous history rather than guessing how much to subtract.
func correction486AccountingPlan(tx *sqlx.Tx) (correction486Plan, error) {
	var p correction486Plan
	err := tx.Select(&p.Events, `SELECT o.id,o.earn_log_id,o.beneficiary_user_id,o.commission_kind,CAST(o.commission_usd AS CHAR) AS usd,o.status,e.user_id AS source_user,e.amount AS source_amount,CAST(e.created_at AS CHAR) AS event_at,COALESCE(o.remote_payment_id,'') AS remote_id,COALESCE(CAST(o.signet_amount AS CHAR),'') AS signet_amount FROM blockchain_commission_outbox o JOIN earn_logs e ON e.id=o.earn_log_id WHERE o.purchase_intent_id=29 ORDER BY o.id FOR UPDATE`)
	if err != nil {
		return p, err
	}
	expected := []struct {
		ID, Earn, User    uint64
		Kind, USD, Status string
	}{{123, 1193, 471, "gratitude", "200", "confirmed"}, {124, 1194, 377, "direct_team", "500", "confirmed"}, {125, 1195, 1, "direct_team", "500", "accounting_only"}, {126, 1196, 377, "leadership", "50", "confirmed"}, {127, 1197, 1, "leadership", "50", "accounting_only"}, {128, 1198, 378, "executive", "50", "confirmed"}}
	if len(p.Events) != len(expected) {
		return p, fmt.Errorf("exact six original source earnings required")
	}
	nativeIDs := []string{"77", "78", "", "79", "", "80"}
	quantities := []string{"14221.815109418335022772", "35554.537773545837556930", "", "3555.453777354583755693", "", "3555.453777354583755693"}
	for i, r := range p.Events {
		e := expected[i]
		a, ok := newRat(r.USD)
		b, ok2 := newRat(r.SourceAmount)
		if r.Remote != nativeIDs[i] || r.Tokens != quantities[i] || !ok || !ok2 || r.ID != e.ID || r.Earn != e.Earn || r.User != e.User || r.Kind != e.Kind || r.Status != e.Status || a != e.USD || b != e.USD || r.SourceUser != strconv.FormatUint(e.User, 10) || r.At == "" || r.At != p.Events[0].At {
			return p, fmt.Errorf("source earning identity or original event timestamp changed")
		}
	}
	var global int
	if err = tx.Get(&global, `SELECT COUNT(*) FROM global_share_wallets_log WHERE user_id IN ('377','378','471') AND created_at=?`, p.Events[0].At); err != nil {
		return p, err
	}
	if global != 0 {
		return p, fmt.Errorf("original event has unbound global-share credits; separate attribution audit required before any hold or recovery")
	}
	for _, r := range []struct {
		User uint64
		USD  string
	}{{1, "550"}, {377, "550"}, {378, "50"}, {471, "200"}} {
		var identity struct {
			ID   uint64 `db:"id"`
			UUID string `db:"uuid"`
		}
		if err = tx.Get(&identity, "SELECT id,COALESCE(uuid,'') AS uuid FROM users WHERE id=? FOR UPDATE", r.User); err != nil {
			return p, err
		}
		uuids := map[uint64]string{377: "019f2876-4760-7000-8000-000000000179", 378: "019f287f-fbc0-7000-8000-00000000017a", 471: "121210a4-45da-4681-ad22-a11ac2761427"}
		if identity.ID != r.User || (r.User != 1 && identity.UUID != uuids[r.User]) {
			return p, fmt.Errorf("source recipient identity changed")
		}

		var w correction486Wallet
		if err = tx.Get(&w, "SELECT id,CAST(balance AS CHAR) AS balance FROM wallets WHERE user_id=? FOR UPDATE", r.User); err != nil {
			return p, err
		}
		w.User = r.User
		w.Delta = r.USD
		if _, err = correction486Subtract(w.Before, w.Delta, 18); err != nil {
			return p, fmt.Errorf("source USD wallet %d cannot cover reversal", r.User)
		}
		p.Wallets = append(p.Wallets, w)
	}
	// 377's two credits have corroborated secondary company overflow rows: those
	// credits did NOT increment that user's first-package earn field. Only 378/471
	// require a package-earn offset. The company bookkeeping USD 550 is also undone.
	for _, r := range []struct {
		User uint64
		USD  string
	}{{378, "50"}, {471, "200"}} {
		var e correction486Earn
		if err = tx.Get(&e, "SELECT id,COALESCE(earn,'0') AS earn,CAST(created_at AS CHAR) AS created_at FROM user_packages WHERE user_id=? ORDER BY id LIMIT 1 FOR UPDATE", r.User); err != nil {
			return p, err
		}
		e.User = r.User
		e.Delta = r.USD
		if e.Created == "" || e.Created > p.Events[0].At {
			return p, fmt.Errorf("original first-package earning owner changed")
		}
		if _, err = correction486Subtract(e.Before, e.Delta, 18); err != nil {
			return p, fmt.Errorf("source package earnings cannot cover reversal for %d", r.User)
		}
		p.Packages = append(p.Packages, e)
	}
	return p, nil
}
func correction486Subtract(a, b string, scale int) (string, error) {
	x, ok := new(big.Rat).SetString(a)
	y, ok2 := new(big.Rat).SetString(b)
	if !ok || !ok2 || y.Sign() <= 0 || x.Cmp(y) < 0 {
		return "", fmt.Errorf("invalid or insufficient exact-decimal source balance")
	}
	difference := new(big.Rat).Sub(x, y)
	text := difference.FloatString(scale)
	exact, _ := new(big.Rat).SetString(text)
	if exact.Cmp(difference) != 0 {
		return "", fmt.Errorf("source precision would be lost; manual review required")
	}
	return text, nil
}

type correction486Recovered struct {
	ID     int    `json:"commission_id"`
	Amount string `json:"amount"`
	Hash   string `json:"tx_hash"`
}
type correction486Completion struct {
	Environment string                   `json:"environment"`
	Operation   string                   `json:"operation"`
	Buyer       string                   `json:"buyer_uuid"`
	Status      string                   `json:"status"`
	Issued      int64                    `json:"issued_at"`
	Expires     int64                    `json:"expires_at"`
	Rows        []correction486Recovered `json:"commissions"`
}

func correction486ReadCompletion(raw []byte, key string, now time.Time) (correction486Completion, error) {
	var p correction486Completion
	var envelope struct {
		Payload   string `json:"payload"`
		Signature string `json:"signature"`
	}
	if len(key) < 32 || len(raw) > 1048576 || json.Unmarshal(raw, &envelope) != nil {
		return p, fmt.Errorf("invalid completion proof")
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte("placement-correction-recovered-v1|" + envelope.Payload))
	sig, err := hex.DecodeString(envelope.Signature)
	if err != nil || !hmac.Equal(mac.Sum(nil), sig) {
		return p, fmt.Errorf("invalid recovery completion signature")
	}
	b, err := base64.StdEncoding.DecodeString(envelope.Payload)
	if err != nil || json.Unmarshal(b, &p) != nil {
		return p, fmt.Errorf("invalid completion payload")
	}
	if p.Environment != "production" || p.Operation != Hold486Key || p.Buyer != Hold486UUID || p.Status != "recovered" || p.Issued > now.Unix()+60 || p.Expires <= now.Unix() || p.Expires-p.Issued != 1800 || len(p.Rows) != 4 {
		return p, fmt.Errorf("expired or wrong-scope recovery completion")
	}
	expected := []struct {
		ID     int
		Amount string
	}{{77, "14221.815109418335022772"}, {78, "35554.537773545837556930"}, {79, "3555.453777354583755693"}, {80, "3555.453777354583755693"}}
	seen := map[string]bool{}
	for i, r := range p.Rows {
		if r.ID != expected[i].ID || r.Amount != expected[i].Amount || !regexp.MustCompile(`^0x[a-f0-9]{64}$`).MatchString(r.Hash) || seen[r.Hash] {
			return p, fmt.Errorf("exact four distinct confirmed correction transfers required")
		}
		seen[r.Hash] = true
	}
	return p, nil
}
func Finalize486(db *sqlx.DB, raw []byte, key string, now time.Time) (map[string]interface{}, error) {
	p, err := correction486ReadCompletion(raw, key, now)
	if err != nil {
		return nil, err
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var buyer struct {
		UUID   string `db:"uuid"`
		Status string `db:"status"`
	}
	if err = tx.Get(&buyer, "SELECT uuid,status FROM users WHERE id=486 FOR UPDATE"); err != nil {
		return nil, err
	}
	if buyer.UUID != Hold486UUID || buyer.Status != "inactive" {
		return nil, fmt.Errorf("buyer must remain inactive during correction")
	}
	var pkg string
	if err = tx.Get(&pkg, "SELECT status FROM user_packages WHERE id=521 AND user_id=486 FOR UPDATE"); err != nil {
		return nil, err
	}
	if pkg != "deactive" && pkg != "deactivate" {
		return nil, fmt.Errorf("buyer package hold changed")
	}
	var row struct {
		Evidence   string         `db:"evidence"`
		Status     string         `db:"status"`
		Completion sql.NullString `db:"completion"`
	}
	if err = tx.Get(&row, "SELECT evidence,status,completion FROM placement_correction_holds WHERE operation_key=? FOR UPDATE", Hold486Key); err != nil {
		return nil, err
	}
	if row.Status == "recovered" {
		var saved struct {
			Rows []correction486Recovered `json:"commissions"`
		}
		if json.Unmarshal([]byte(row.Completion.String), &saved) != nil || !reflect.DeepEqual(saved.Rows, p.Rows) {
			return nil, fmt.Errorf("recovery completion changed")
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "recovered", "source_accounting_reversed": true, "buyer_still_held": true, "funds_sent": false}, nil
	}
	if row.Status != "held" {
		return nil, fmt.Errorf("source correction state requires reconciliation")
	}
	var saved struct {
		Plan correction486Plan `json:"accounting_plan"`
	}
	if json.Unmarshal([]byte(row.Evidence), &saved) != nil || len(saved.Plan.Events) != 6 {
		return nil, fmt.Errorf("source hold lacks reviewed accounting plan; reconcile before reversal")
	}
	plan, err := correction486AccountingPlan(tx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(saved.Plan.Events, plan.Events) {
		return nil, fmt.Errorf("original source accounting history changed")
	}
	for i, w := range plan.Wallets {
		if len(saved.Plan.Wallets) != 4 || w.ID != saved.Plan.Wallets[i].ID || w.User != saved.Plan.Wallets[i].User {
			return nil, fmt.Errorf("source wallet ownership changed")
		}
	}
	for i, e := range plan.Packages {
		if len(saved.Plan.Packages) != 2 || e.ID != saved.Plan.Packages[i].ID || e.User != saved.Plan.Packages[i].User {
			return nil, fmt.Errorf("first-package source changed")
		}
	}
	for _, w := range plan.Wallets {
		after, _ := correction486Subtract(w.Before, w.Delta, 18)
		updated, e := tx.Exec("UPDATE wallets SET balance=?,updated_at=NOW() WHERE id=? AND user_id=?", after, w.ID, w.User)
		if e != nil {
			return nil, e
		}
		count, e := updated.RowsAffected()
		if e != nil || count != 1 {
			return nil, fmt.Errorf("source wallet reversal not persisted")
		}

	}
	for _, e := range plan.Packages {
		after, _ := correction486Subtract(e.Before, e.Delta, 18)
		updated, failure := tx.Exec("UPDATE user_packages SET earn=?,updated_at=NOW() WHERE id=? AND user_id=?", after, e.ID, e.User)
		if failure != nil {
			return nil, failure
		}
		count, failure := updated.RowsAffected()
		if failure != nil || count != 1 {
			return nil, fmt.Errorf("source package earning reversal not persisted")
		}

	}
	ids := []int64{}
	for _, e := range plan.Events {
		amount, _ := newRat(e.USD)
		r, err := tx.Exec("INSERT INTO earn_logs (user_id,amount,description,created_at,updated_at) VALUES (?,?,?,NOW(),NOW())", e.SourceUser, "-"+amount, fmt.Sprintf("Placement 486 reversal of earn_log %d (intent 29)", e.Earn))
		if err != nil {
			return nil, err
		}
		id, err := r.LastInsertId()
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("source reversal log not persisted")
		}
		ids = append(ids, id)
	}
	completion, _ := json.Marshal(map[string]interface{}{"commissions": p.Rows, "source_before": plan, "negative_earn_log_ids": ids, "completed_at": now.UTC().Format(time.RFC3339)})
	updated, err := tx.Exec("UPDATE placement_correction_holds SET status='recovered',completion=? WHERE operation_key=? AND status='held'", string(completion), Hold486Key)
	if err != nil {
		return nil, err
	}
	count, err := updated.RowsAffected()
	if err != nil || count != 1 {
		return nil, fmt.Errorf("source correction completion not persisted")
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "recovered", "source_accounting_reversed": true, "buyer_still_held": true, "funds_sent": false}, nil
}

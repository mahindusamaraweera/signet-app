package handlers

import (
	"database/sql"
	"fmt"
	"math/big"
	"regexp"
	"signet-backend/internal/wallet"

	"github.com/jmoiron/sqlx"
)

// Bind every integration commission to the exact legacy earn log created by
// this transaction. No current-price recomputation or chain transfer occurs here.
type commissionRecorder func(tx *sqlx.Tx, earnLogID int64, userID uint64, amount, description string) error

var commissionDecimal = regexp.MustCompile(`^[0-9]{1,18}(\.[0-9]{1,18})?$`)

type activationCommissionContext struct{ IntentID uint64 }

// Integration-only routing, using the unchanged USD 4x cap basis and the
// actual commission for this role. Manual legacy callers are unchanged.
func creditActivationCommissionTx(tx *sqlx.Tx, userID uint64, amount float64, description, kind string, ctx activationCommissionContext) error {
	if ctx.IntentID == 0 {
		return fmt.Errorf("missing purchase intent")
	}
	recorder := purchaseCommissionRecorder(ctx.IntentID, kind)
	if userID == 1 {
		return creditDivertedCommissionTx(tx, amount, recorder)
	}
	value, ok := new(big.Rat).SetString(ftoaF(amount))
	if !ok || value.Sign() < 0 {
		return fmt.Errorf("invalid commission")
	}
	if value.Sign() == 0 {
		return nil
	}
	var id uint64
	if err := tx.Get(&id, "SELECT id FROM users WHERE id=? FOR UPDATE", userID); err != nil {
		return err
	}
	var balance sql.NullFloat64
	err := tx.Get(&balance, "SELECT balance FROM wallets WHERE user_id=? FOR UPDATE", userID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	var cap sql.NullFloat64
	if err := tx.Get(&cap, `SELECT COALESCE(SUM(p.price*4),0) - (SELECT COALESCE(SUM(commission_usd),0) FROM blockchain_topup_commission_plans WHERE beneficiary_user_id=? AND status='reserved') FROM user_packages up JOIN packages p ON p.id=CAST(up.package AS UNSIGNED) WHERE up.user_id=? AND up.status='active'`, userID, userID); err != nil {
		return err
	}
	if wallet.CommissionFitsCap(balance.Float64, amount, cap.Float64) {
		return creditWalletTx(tx, userID, amount, description, recorder)
	}
	if err := creditDivertedCommissionTx(tx, amount, recorder); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO income_cap_diversions (user_id,purchase_intent_id,commission_usd,commission_kind) VALUES (?,?,?,?)`, userID, ctx.IntentID, ftoaF(amount), kind)
	return err
}

func purchaseCommissionRecorder(intentID uint64, kinds ...string) commissionRecorder {
	kind := "unclassified"
	if len(kinds) > 0 {
		kind = kinds[0]
	}
	return func(tx *sqlx.Tx, earnLogID int64, userID uint64, amount, description string) error {
		value, ok := new(big.Rat).SetString(amount)
		if !commissionDecimal.MatchString(amount) || !ok || value.Sign() < 0 || earnLogID <= 0 || userID == 0 || intentID == 0 {
			return fmt.Errorf("invalid commission evidence")
		}
		if value.Sign() == 0 {
			return nil
		}
		_, err := tx.Exec(`INSERT INTO blockchain_commission_outbox (purchase_intent_id,earn_log_id,beneficiary_user_id,commission_usd,description,commission_kind) VALUES (?,?,?,?,?,?)`, intentID, earnLogID, userID, amount, description, kind)
		if err == nil && (kind == "unclassified" || (userID == 1 && description != "Income cap commission diversion")) {
			// Legacy creditWalletTx can write an extra company accounting row.
			// Preserve its history, but never equate it with a second treasury
			// entitlement without explicit reconciliation of that legacy rule.
			_, err = tx.Exec(`UPDATE blockchain_commission_outbox SET status='requires_reconciliation' WHERE earn_log_id=?`, earnLogID)
		}
		return err
	}
}

// Company diversion is one credit, not a second application of user earnings
// overflow/global-share rules. A separate outbox remains awaiting payout policy.
func creditDivertedCommissionTx(tx *sqlx.Tx, amount float64, recorders ...commissionRecorder) error {
	value := ftoaF(amount)
	parsed, ok := new(big.Rat).SetString(value)
	if !commissionDecimal.MatchString(value) || !ok || parsed.Sign() < 0 {
		return fmt.Errorf("invalid diverted commission")
	}
	if parsed.Sign() == 0 {
		return nil
	}
	var companyID uint64
	if err := tx.Get(&companyID, "SELECT id FROM users WHERE id=1 FOR UPDATE"); err != nil {
		return err
	}
	var walletID uint64
	err := tx.Get(&walletID, "SELECT id FROM wallets WHERE user_id=1 FOR UPDATE")
	if err == sql.ErrNoRows {
		_, err = tx.Exec("INSERT INTO wallets (user_id,balance,created_at,updated_at) VALUES (1,?,NOW(),NOW())", amount)
	} else if err == nil {
		_, err = tx.Exec("UPDATE wallets SET balance=balance+?,updated_at=NOW() WHERE id=?", amount, walletID)
	}
	if err != nil {
		return err
	}
	result, err := tx.Exec(`INSERT INTO earn_logs (user_id,amount,description,created_at,updated_at) VALUES ('1',?,'Income cap commission diversion',NOW(),NOW())`, value)
	if err != nil {
		return err
	}
	logID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	for _, record := range recorders {
		if err := record(tx, logID, 1, value, "Income cap commission diversion"); err != nil {
			return err
		}
	}
	return nil
}

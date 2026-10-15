package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"log"
	"math/big"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"time"
)

// This closes accounting evidence only. Custody owns treasury swaps/receipts.
func StartCompanySettlement(d *app.Deps) {
	if d.Blockchain == nil || d.Cfg == nil || !d.Cfg.BlockchainCompanySettlementEnabled || !d.Cfg.BlockchainCommissionPreparationEnabled || !d.Cfg.BlockchainCommissionDeliveryEnabled {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			var ids []uint64
			err := d.DB.Select(&ids, `SELECT p.id FROM package_purchase_intents p LEFT JOIN blockchain_company_settlements c ON c.purchase_intent_id=p.id WHERE p.status='completed' AND p.purchase_kind='first' AND (c.status IS NULL OR c.status IN ('pending','sealed','skipped')) ORDER BY COALESCE(c.last_checked_at,p.created_at),p.id LIMIT 50`)
			if err != nil {
				log.Printf("company settlement queue: %v", err)
			} else {
				for _, id := range ids {
					if err := syncCompanySettlement(d, id); err != nil {
						log.Printf("company settlement %d: %v", id, err)
					}
				}
			}
			<-ticker.C
		}
	}()
}

func syncCompanySettlement(d *app.Deps, id uint64) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var intent blockchain.PurchaseIntent
	err = tx.Get(&intent, `SELECT id,user_id,package_price,purchase_kind FROM package_purchase_intents WHERE id=? AND status='completed' AND purchase_kind='first' FOR UPDATE`, id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	// Commit the attempt before contacting custody so ambiguous network errors
	// cannot starve other purchases; next attempt uses the same manifest identity.
	_, err = tx.Exec(`INSERT INTO blockchain_company_settlements (purchase_intent_id,last_checked_at) VALUES (?,NOW()) ON DUPLICATE KEY UPDATE last_checked_at=NOW()`, id)
	if err != nil {
		return err
	}
	var state string
	if err = tx.Get(&state, "SELECT status FROM blockchain_company_settlements WHERE purchase_intent_id=? FOR UPDATE", id); err != nil {
		return err
	}
	if state == "confirmed" {
		return tx.Commit()
	}
	// The reviewed legacy production order is already paid in custody. Read its
	// audited receipt instead of resealing UUID terms or replaying any payout.
	if d.Cfg != nil && d.Cfg.AppEnv == "production" && intent.UserID == 460 && id == 1 {
		price, valid := new(big.Rat).SetString(intent.Price)
		if !valid || price.Cmp(big.NewRat(1000, 1)) != 0 {
			return fmt.Errorf("legacy settlement source price changed")
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		receipt, err := d.Blockchain.Recovered460CompanyReceipt(ctx)
		if err != nil {
			return err
		}
		if !*receipt.Settled {
			return nil
		}
		_, err = d.DB.Exec(`UPDATE blockchain_company_settlements SET status='confirmed',manifest_hash=?,updated_at=NOW() WHERE purchase_intent_id=1 AND status IN ('pending','sealed','skipped')`, receipt.ManifestHash)
		return err
	}
	var rows []struct {
		blockchain.CommissionInstruction
		Status string `db:"status"`
	}
	if err = tx.Select(&rows, `SELECT id,earn_log_id,purchase_intent_id,beneficiary_user_id,commission_kind,commission_usd,status FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id`, id); err != nil {
		return err
	}
	for _, row := range rows {
		if row.Status == "requires_reconciliation" && row.RecipientID == 1 {
			if err := classifyLegacyCompanyAccounting(tx, id); err != nil {
				return err
			}
			rows = nil
			if err := tx.Select(&rows, `SELECT id,earn_log_id,purchase_intent_id,beneficiary_user_id,commission_kind,commission_usd,status FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id`, id); err != nil {
				return err
			}
			break
		}
	}
	commissions := make([]blockchain.CommissionInstruction, 0, len(rows))
	for _, row := range rows {
		switch row.Status {
		case "accounting_only":
			if row.RecipientID != 1 {
				return fmt.Errorf("non-company commission marked accounting-only")
			}
			continue
		case "awaiting_authorization", "authorized", "confirming", "confirmed":
			row.BuyerID = intent.UserID
			commissions = append(commissions, row.CommissionInstruction)
		default:
			return tx.Commit() // Unprepared or disputed evidence forbids settlement.
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := d.Blockchain.SealFirstCompanyManifest(ctx, intent, commissions)
	if err != nil {
		return err
	}
	state = "sealed"
	if !*result.Enabled {
		state = "skipped"
	} else if *result.Settled {
		state = "confirmed"
	}
	updated, err := d.DB.Exec(`UPDATE blockchain_company_settlements SET status=?,manifest_hash=NULLIF(?,''),updated_at=NOW() WHERE purchase_intent_id=? AND status IN ('pending','sealed','skipped')`, state, result.Hash, id)
	if err != nil {
		return err
	}
	n, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if n > 1 {
		return fmt.Errorf("company settlement identity changed")
	}
	return nil
}

// Reclassify only unsubmitted secondary company entries corroborated by a
// confirmed non-company payout and both original earn logs. Financial credits,
// payout amounts and transaction identities are never changed by this repair.
// Legacy tables may have different utf8mb4 collations. Compare textual evidence
// as exact bytes so matching neither fails on collation nor folds case/accents.
func classifyLegacyCompanyAccounting(tx *sqlx.Tx, intentID uint64) error {
	_, err := tx.Exec(`UPDATE blockchain_commission_outbox extra
 JOIN earn_logs company_log ON company_log.id=extra.earn_log_id
 JOIN blockchain_commission_outbox paid ON paid.purchase_intent_id=extra.purchase_intent_id
  AND paid.commission_kind=extra.commission_kind AND paid.commission_usd=extra.commission_usd
  AND CAST(paid.description AS BINARY)=CAST(extra.description AS BINARY) AND paid.beneficiary_user_id<>1
 JOIN earn_logs paid_log ON paid_log.id=paid.earn_log_id
 SET extra.status='accounting_only'
 WHERE extra.purchase_intent_id=? AND extra.beneficiary_user_id=1
  AND extra.status='requires_reconciliation' AND extra.commission_kind IN ('direct_team','gratitude','spillover','leadership','executive')
  AND extra.description<>'Income cap commission diversion'
  AND extra.remote_payment_id IS NULL AND extra.transfer_tx_hash IS NULL AND extra.signet_amount IS NULL
  AND paid.status='confirmed' AND paid.transfer_tx_hash IS NOT NULL AND paid.transfer_tx_hash<>''
  AND CAST(company_log.user_id AS BINARY)=CAST('1' AS BINARY) AND CAST(paid_log.user_id AS BINARY)=CAST(CAST(paid.beneficiary_user_id AS CHAR) AS BINARY)
  AND company_log.id>paid_log.id AND company_log.created_at=paid_log.created_at
  AND CAST(company_log.amount AS DECIMAL(36,18))=extra.commission_usd
  AND CAST(paid_log.amount AS DECIMAL(36,18))=paid.commission_usd
  AND CAST(COALESCE(company_log.description,'') AS BINARY)=CAST(extra.description AS BINARY)
  AND CAST(COALESCE(paid_log.description,'') AS BINARY)=CAST(paid.description AS BINARY)`, intentID)
	return err
}

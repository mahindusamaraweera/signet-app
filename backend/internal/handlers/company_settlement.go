package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	var rows []struct {
		blockchain.CommissionInstruction
		Status string `db:"status"`
	}
	if err = tx.Select(&rows, `SELECT id,earn_log_id,purchase_intent_id,beneficiary_user_id,commission_kind,commission_usd,status FROM blockchain_commission_outbox WHERE purchase_intent_id=? ORDER BY id`, id); err != nil {
		return err
	}
	commissions := make([]blockchain.CommissionInstruction, 0, len(rows))
	for _, row := range rows {
		switch row.Status {
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

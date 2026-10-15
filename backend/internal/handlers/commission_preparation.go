package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"time"
)

// Preparation only: this worker cannot sign, credit SIGNET or declare a payout.
func StartCommissionPreparation(d *app.Deps) {
	if d.Blockchain == nil || d.Cfg == nil || !d.Cfg.BlockchainCommissionPreparationEnabled {
		return
	}
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			var ids []uint64
			if err := d.DB.Select(&ids, `SELECT o.id FROM blockchain_commission_outbox o JOIN package_purchase_intents p ON p.id=o.purchase_intent_id WHERE o.status='awaiting_policy' AND p.status='completed' AND o.commission_kind IN ('direct_team','gratitude','spillover','leadership','executive') ORDER BY COALESCE(o.last_checked_at,o.created_at),o.id LIMIT 50`); err != nil {
				log.Printf("commission preparation queue: %v", err)
			} else {
				runPurchaseChecks(ids, func(id uint64) error { return syncCommissionPreparation(d, id) }, func(id uint64, err error) {
					log.Printf("commission preparation %d: %v", id, err)
					if _, retryErr := d.DB.Exec("UPDATE blockchain_commission_outbox SET last_checked_at=NOW() WHERE id=? AND status='awaiting_policy'", id); retryErr != nil {
						log.Printf("commission preparation retry timestamp %d: %v", id, retryErr)
					}
				})
			}
			<-ticker.C
		}
	}()
}

func syncCommissionPreparation(d *app.Deps, id uint64) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var instruction blockchain.CommissionInstruction
	err = tx.Get(&instruction, `SELECT o.id,o.earn_log_id,o.purchase_intent_id,p.user_id AS buyer_id,o.beneficiary_user_id,o.commission_kind,o.commission_usd FROM blockchain_commission_outbox o JOIN package_purchase_intents p ON p.id=o.purchase_intent_id WHERE o.id=? AND o.status='awaiting_policy' AND p.status='completed' AND o.commission_kind IN ('direct_team','gratitude','spillover','leadership','executive') FOR UPDATE`, id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := d.Blockchain.PrepareCommission(ctx, instruction)
	if err != nil {
		return err
	}
	var quote struct {
		Price  string `json:"signet_price_usdt"`
		Amount string `json:"signet_amount"`
	}
	if err := json.Unmarshal(result.Quote, &quote); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE blockchain_commission_outbox SET status='awaiting_authorization',remote_payment_id=?,signet_price_usdt=?,signet_amount=?,recipient_address=?,quote_snapshot=?,last_checked_at=NOW() WHERE id=?`, result.ID, quote.Price, quote.Amount, result.RecipientAddress, string(result.Quote), id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

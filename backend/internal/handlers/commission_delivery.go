package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func StartCommissionDelivery(d *app.Deps) {
	if d.Blockchain == nil || d.Cfg == nil || !d.Cfg.BlockchainCommissionDeliveryEnabled {
		return
	}
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			var ids []uint64
			err := d.DB.Select(&ids, `SELECT o.id FROM blockchain_commission_outbox o JOIN package_purchase_intents p ON p.id=o.purchase_intent_id WHERE o.status IN ('awaiting_authorization','authorized','confirming') AND p.status='completed' AND p.purchase_kind='first' ORDER BY COALESCE(o.last_checked_at,o.created_at),o.id LIMIT 50`)
			if err != nil {
				log.Printf("commission delivery queue: %v", err)
			} else {
				runPurchaseChecks(ids, func(id uint64) error { return syncCommissionDelivery(d, id) }, func(id uint64, err error) {
					log.Printf("commission delivery %d: %v", id, err)
					if _, retryErr := d.DB.Exec("UPDATE blockchain_commission_outbox SET last_checked_at=NOW() WHERE id=? AND status IN ('awaiting_authorization','authorized','confirming')", id); retryErr != nil {
						log.Printf("commission delivery retry timestamp %d: %v", id, retryErr)
					}
				})
			}
			<-ticker.C
		}
	}()
}

func syncCommissionDelivery(d *app.Deps, id uint64) error {
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var row struct {
		blockchain.CommissionInstruction
		RemoteID string `db:"remote_payment_id"`
		Address  string `db:"recipient_address"`
	}
	err = tx.Get(&row, `SELECT o.id,o.earn_log_id,o.purchase_intent_id,p.user_id AS buyer_id,o.beneficiary_user_id,o.commission_kind,o.commission_usd,o.remote_payment_id,o.recipient_address FROM blockchain_commission_outbox o JOIN package_purchase_intents p ON p.id=o.purchase_intent_id WHERE o.id=? AND o.status IN ('awaiting_authorization','authorized','confirming') AND p.status='completed' AND p.purchase_kind='first' FOR UPDATE`, id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	result, err := d.Blockchain.AuthorizeCommission(ctx, row.CommissionInstruction)
	if err != nil {
		return err
	}
	if result.ID != row.RemoteID || !strings.EqualFold(result.RecipientAddress, row.Address) {
		return fmt.Errorf("commission authorization identity changed")
	}
	var quote struct {
		Price  string `json:"signet_price_usdt"`
		Amount string `json:"signet_amount"`
	}
	if err := json.Unmarshal(result.Quote, &quote); err != nil {
		return err
	}
	// Source USD/cap accounting was already committed during activation.
	// This records custody delivery only; never credit a source wallet again.
	_, err = tx.Exec(`UPDATE blockchain_commission_outbox SET status=?,signet_price_usdt=?,signet_amount=?,quote_snapshot=?,transfer_tx_hash=NULLIF(?,''),last_checked_at=NOW() WHERE id=?`, result.Status, quote.Price, quote.Amount, string(result.Quote), result.TxHash, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

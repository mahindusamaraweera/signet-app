package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/withdrawalsecurity"
	"strconv"
	"time"
)

func StartWithdrawalCancellation(d *app.Deps) {
	if d.Cfg == nil || !d.Cfg.BlockchainWithdrawalCancellationEnabled || d.Blockchain == nil || d.WithdrawalSecurity == nil {
		return
	}
	key, err := d.Cfg.WithdrawalAuthorizationKey()
	if err != nil {
		log.Print("withdrawal cancellation disabled: independent key required")
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			var rows []struct {
				ID    string `db:"authorization_id"`
				Terms string `db:"terms"`
			}
			if err := d.DB.Select(&rows, `SELECT authorization_id,terms FROM withdrawal_submission_outbox WHERE status='cancel_requested' ORDER BY updated_at,authorization_id LIMIT 10`); err != nil {
				log.Print("withdrawal cancellation queue unavailable")
			} else {
				for _, row := range rows {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					var terms withdrawalsecurity.WithdrawalTerms
					err := json.Unmarshal([]byte(row.Terms), &terms)
					if err == nil {
						err = cancelQueuedWithdrawal(ctx, d, row.ID, terms, key)
					}
					cancel()
					if err != nil {
						log.Print("withdrawal cancellation pending; retry or reconciliation required")
						if _, err := d.DB.Exec(`UPDATE withdrawal_submission_outbox SET attempts=attempts+1,last_error='Cancellation pending; retry or reconciliation required',updated_at=NOW() WHERE authorization_id=? AND status='cancel_requested'`, row.ID); err != nil {
							log.Print("withdrawal cancellation retry record unavailable")
						}
					}
				}
			}
			<-ticker.C
		}
	}()
}
func cancelQueuedWithdrawal(ctx context.Context, d *app.Deps, id string, terms withdrawalsecurity.WithdrawalTerms, key []byte) error {
	assertion, signature, err := d.WithdrawalSecurity.BuildWithdrawalCancellation(ctx, id, terms, key, time.Now().UTC())
	if err != nil {
		return err
	}
	result, err := d.Blockchain.CancelWithdrawal(ctx, assertion, signature)
	if err != nil {
		if errors.Is(err, blockchain.ErrCancellationReviewRequired) {
			return d.WithdrawalSecurity.RecordWithdrawalCancellationReview(ctx, id, terms, time.Now().UTC())
		}
		return err
	}
	var remote *uint64
	if result.WithdrawalID != nil {
		value, err := strconv.ParseUint(*result.WithdrawalID, 10, 64)
		if err != nil {
			return err
		}
		remote = &value
	}
	return d.WithdrawalSecurity.RecordWithdrawalCancellation(ctx, id, terms, remote, time.Now().UTC())
}

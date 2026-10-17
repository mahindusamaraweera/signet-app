package handlers

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/withdrawalsecurity"
)

func StartWithdrawalSubmission(d *app.Deps) {
	if d.Cfg == nil || !d.Cfg.BlockchainWithdrawalSubmissionEnabled || d.Blockchain == nil || d.WithdrawalSecurity == nil {
		return
	}
	key, err := d.Cfg.WithdrawalAuthorizationKey()
	if err != nil {
		log.Print("withdrawal submission disabled: independent approval key required")
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
			if err := d.DB.Select(&rows, `SELECT authorization_id,terms FROM withdrawal_submission_outbox WHERE status='pending' ORDER BY updated_at,authorization_id LIMIT 10`); err != nil {
				log.Print("withdrawal submission queue unavailable")
			} else {
				for _, row := range rows {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					var terms withdrawalsecurity.WithdrawalTerms
					err := json.Unmarshal([]byte(row.Terms), &terms)
					if err == nil {
						err = submitQueuedWithdrawal(ctx, d, row.ID, terms, key)
					}
					cancel()
					if err != nil {
						// Never persist signatures, assertion contents or remote error bodies.
						diagnostic := withdrawalDiagnostic(err)
						log.Printf("withdrawal reservation pending: %s authorization_id=%s", diagnostic, row.ID)
						if _, recordErr := d.DB.Exec(`UPDATE withdrawal_submission_outbox SET attempts=attempts+1,last_error=?,updated_at=NOW() WHERE authorization_id=? AND status='pending'`, diagnostic, row.ID); recordErr != nil {
							log.Print("withdrawal retry record unavailable")
						}
					}
				}
			}
			<-ticker.C
		}
	}()
}

func submitQueuedWithdrawal(ctx context.Context, d *app.Deps, id string, terms withdrawalsecurity.WithdrawalTerms, key []byte) error {
	a, signature, err := d.WithdrawalSecurity.BuildQueuedWithdrawalAssertion(ctx, id, terms, key, time.Now().UTC())
	if err != nil {
		return withdrawalStage("build_assertion", err)
	}
	remote, err := d.Blockchain.ReserveWithdrawal(ctx, a, signature)
	if err != nil {
		return withdrawalStage("token_reservation", err)
	}
	remoteID, err := strconv.ParseUint(remote.ID, 10, 64)
	if err != nil {
		return withdrawalStage("validate_response", err)
	}
	return withdrawalStage("record_reservation", d.WithdrawalSecurity.RecordWithdrawalReservation(ctx, id, terms, remoteID, time.Now().UTC()))
}

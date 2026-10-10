package handlers

import (
	"errors"
	"log"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

func reopeningFailureState(err error) string {
	if errors.Is(err, blockchain.ErrConflict) || errors.Is(err, blockchain.ErrRejected) {
		return "review_required"
	}
	return "pending"
}

// Default-off registration expiry gate; retries only already committed intents.
func StartRegistrationReopening(d *app.Deps) {
	if d.Cfg == nil || !d.Cfg.BlockchainEnabled || !d.Cfg.RegistrationExpiryEnabled || d.Blockchain == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			var ids []uint64
			if err := d.DB.Select(&ids, "SELECT purchase_intent_id FROM registration_reopening_outbox WHERE status='pending' ORDER BY COALESCE(last_attempt_at,created_at),purchase_intent_id LIMIT 10"); err != nil {
				log.Print("registration reopening queue unavailable")
			} else {
				for _, id := range ids {
					if _, err := d.DB.Exec("UPDATE registration_reopening_outbox SET attempts=attempts+1,last_attempt_at=NOW() WHERE purchase_intent_id=? AND status='pending'", id); err != nil {
						continue
					}
					if err := syncRegistrationReopening(d, id); err != nil {
						_, recordErr := d.DB.Exec("UPDATE registration_reopening_outbox SET status=?,last_error='Reopening not confirmed; funding remains blocked' WHERE purchase_intent_id=? AND status='pending'", reopeningFailureState(err), id)
						if recordErr != nil {
							log.Print("registration reopening retry record unavailable")
						}
					}
				}
			}
			<-ticker.C
		}
	}()
}

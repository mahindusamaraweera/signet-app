package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/blockchain"
)

// Does not read vesting options, provision wallets or alter legacy KYC records.
func syncTierIdentity(ctx context.Context, d *app.Deps, userID uint64) (*blockchain.TierKycStatus, error) {
	if d.Blockchain == nil {
		return nil, blockchain.ErrUnavailable
	}
	var linkedID string
	if err := d.DB.GetContext(ctx, &linkedID, "SELECT tier_user_id FROM tier_account_links WHERE user_id=?", userID); err != nil {
		return nil, err
	}
	state, err := d.Blockchain.TierKyc(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !*state.Linked || state.TierUserID == nil || *state.TierUserID != linkedID {
		return nil, blockchain.ErrConflict
	}
	// Serialize cache writes so a late older request cannot restore a revoked decision.
	// The mirror is display-only; withdrawals always recheck Token's authority.
	tx, err := d.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var currentLink string
	if err := tx.GetContext(ctx, &currentLink, "SELECT tier_user_id FROM tier_account_links WHERE user_id=? FOR UPDATE", userID); err != nil || currentLink != linkedID {
		return nil, blockchain.ErrConflict
	}
	var previous time.Time
	err = tx.GetContext(ctx, &previous, "SELECT provider_checked_at FROM tier_identity_verifications WHERE user_id=?", userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && previous.After(state.CheckedAt) {
		return nil, blockchain.ErrUnavailable
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO tier_identity_verifications
 (user_id,tier_user_id,status,verified,environment,level_name,provider_checked_at,valid_until,synced_at)
 VALUES (?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6))
 ON DUPLICATE KEY UPDATE tier_user_id=VALUES(tier_user_id),status=VALUES(status),verified=VALUES(verified),environment=VALUES(environment),level_name=VALUES(level_name),provider_checked_at=VALUES(provider_checked_at),valid_until=VALUES(valid_until),synced_at=UTC_TIMESTAMP(6)`, userID, linkedID, state.Status, *state.Verified, state.Environment, state.Level, state.CheckedAt.UTC(), state.ValidUntil)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return state, nil
}

func StartTierKycSync(d *app.Deps) {
	if d.Blockchain == nil {
		return
	}
	go func() {
		var cursor uint64
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			var ids []uint64
			err := d.DB.Select(&ids, `SELECT l.user_id FROM tier_account_links l LEFT JOIN tier_identity_verifications k ON k.user_id=l.user_id WHERE l.user_id > ? AND (k.user_id IS NULL OR k.synced_at < UTC_TIMESTAMP() - INTERVAL 5 MINUTE) ORDER BY l.user_id LIMIT 100`, cursor)
			if err != nil {
				log.Print("Tier identity synchronization unavailable; check migration and connection")
			} else {
				if len(ids) == 0 {
					cursor = 0
				}
				for _, id := range ids {
					cursor = id
					ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
					_, err := syncTierIdentity(ctx, d, id)
					cancel()
					if err != nil {
						log.Printf("Tier identity status could not be refreshed: user_id=%d", id)
						break
					}
				}
			}
			<-ticker.C
		}
	}()
}

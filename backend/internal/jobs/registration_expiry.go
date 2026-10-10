package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"signet-backend/internal/blockchain"
	"time"
)

func StartRegistrationExpiry(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool) {
	go runOnTicker("registration:expire", time.Minute, func() error { return RunRegistrationExpiry(db, gateway, enabled) })
}

// RunRegistrationExpiry releases unpaid reservations, never user accounts.
func RunRegistrationExpiry(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool) error {
	var ids []uint64
	cutoff := time.Now().Add(-10 * time.Hour)
	if err := db.Select(&ids, `SELECT l.id FROM user_parent_map_logs l JOIN users u ON u.id=l.user_id
		WHERE u.status='pending' AND l.created_at<=? ORDER BY l.id LIMIT 100`, cutoff); err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err := expireRegistration(db, gateway, enabled, id, cutoff); err != nil {
			failures = append(failures, fmt.Errorf("registration expiry %d: %w", id, err))
		}
	}
	return errors.Join(failures...)
}

func expireRegistration(db *sqlx.DB, gateway *blockchain.Gateway, enabled bool, id uint64, cutoff time.Time) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Resolve owner without a lock, then use the same user-first ordering as
	// activation. Re-read the placement under lock before trusting that owner.
	var owner uint64
	if err := tx.Get(&owner, "SELECT user_id FROM user_parent_map_logs WHERE id=?", id); err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	var status string
	if err := tx.Get(&status, "SELECT status FROM users WHERE id=? FOR UPDATE", owner); err != nil {
		return err
	}
	var row struct {
		UserID  uint64    `db:"user_id"`
		Rows    string    `db:"created_row_ids"`
		Created time.Time `db:"created_at"`
	}
	err = tx.Get(&row, "SELECT user_id, created_row_ids, created_at FROM user_parent_map_logs WHERE id=? FOR UPDATE", id)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if row.UserID != owner || status != "pending" || row.Created.After(cutoff) {
		return nil
	}
	var protected int
	err = tx.Get(&protected, `SELECT
		(SELECT COUNT(*) FROM user_packages WHERE user_id=? AND status='active') +
		(SELECT COUNT(*) FROM wallets WHERE user_id=? AND (balance<>0 OR roc_balance<>0)) +
		(SELECT COUNT(*) FROM tokens WHERE user_id=?) +
		(SELECT COUNT(*) FROM user_minings WHERE user_id=? AND (total_token<>0 OR mining_token<>0)) +
		(SELECT COUNT(*) FROM global_share_wallets WHERE user_id=? AND balance<>0)`, row.UserID, row.UserID, row.UserID, row.UserID, row.UserID)
	if err != nil {
		return err
	}
	if protected > 0 {
		return nil
	}
	var rowIDs []uint64
	if err := json.Unmarshal([]byte(row.Rows), &rowIDs); err != nil {
		return err
	}
	if len(rowIDs) == 0 {
		return fmt.Errorf("missing placement rows")
	}
	var custodySeal interface{}
	if enabled {
		var protectedIntent int
		if err := tx.Get(&protectedIntent, `SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=? AND (deposit_seen=TRUE OR status NOT IN ('awaiting_deposit','expired'))`, row.UserID); err != nil {
			return err
		}
		if protectedIntent > 0 {
			return nil
		}
		if gateway == nil {
			return blockchain.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		seal, err := gateway.SealRegistrationExpiry(ctx, row.UserID, id)
		cancel()
		if err != nil {
			return err
		}
		if seal == nil {
			return nil
		}
		evidence, err := json.Marshal(seal)
		if err != nil {
			return err
		}
		custodySeal = string(evidence)
	}
	// Logs can contain other users' rows; remove only this user's inactive placement.
	query, args, err := sqlx.In("DELETE FROM user_parents WHERE user_id=? AND node='deactive' AND id IN (?)", row.UserID, rowIDs)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(tx.Rebind(query), args...); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO registration_expiries (placement_log_id,user_id,created_row_ids,custody_seal) VALUES (?,?,?,?)", id, row.UserID, row.Rows, custodySeal); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM user_parent_map_logs WHERE id=?", id); err != nil {
		return err
	}
	if enabled {
		if _, err := tx.Exec("UPDATE package_purchase_intents SET status='expired' WHERE user_id=? AND status='awaiting_deposit' AND deposit_seen=FALSE", row.UserID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

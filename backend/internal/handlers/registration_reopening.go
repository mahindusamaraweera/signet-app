package handlers

import (
	"context"
	"database/sql"
	"errors"
	"signet-backend/internal/app"
	"time"
)

// Each row must be committed with its replacement placement and fresh intent.
// Remote confirmation is not payment and never credits a source balance.
func syncRegistrationReopening(d *app.Deps, intentID uint64) error {
	var owner uint64
	if err := d.DB.Get(&owner, "SELECT user_id FROM registration_reopening_outbox WHERE purchase_intent_id=?", intentID); err != nil {
		return err
	}
	tx, err := d.DB.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var lockedOwner uint64
	if err := tx.Get(&lockedOwner, "SELECT id FROM users WHERE id=? FOR UPDATE", owner); err != nil {
		return err
	}
	var row struct {
		Owner  uint64 `db:"user_id"`
		Old    uint64 `db:"expired_placement_log_id"`
		New    uint64 `db:"new_placement_log_id"`
		Status string `db:"status"`
	}
	if err := tx.Get(&row, "SELECT user_id,expired_placement_log_id,new_placement_log_id,status FROM registration_reopening_outbox WHERE purchase_intent_id=? FOR UPDATE", intentID); err != nil {
		return err
	}
	if row.Owner != owner || lockedOwner != owner || row.Old == 0 || row.New == 0 || row.Old == row.New {
		return errors.New("invalid reopening identity")
	}
	if row.Status == "confirmed" {
		return nil
	}
	if row.Status != "pending" {
		return errors.New("reopening requires reconciliation")
	}
	var intent struct {
		Owner  uint64 `db:"user_id"`
		Status string `db:"status"`
	}
	if err := tx.Get(&intent, "SELECT user_id,status FROM package_purchase_intents WHERE id=? FOR UPDATE", intentID); err != nil {
		return err
	}
	if intent.Owner != owner || intent.Status != "awaiting_reopening" {
		return errors.New("replacement funding prerequisite mismatch")
	}
	var placementOwner uint64
	if err := tx.Get(&placementOwner, "SELECT user_id FROM user_parent_map_logs WHERE id=? FOR UPDATE", row.New); err != nil {
		return err
	}
	if placementOwner != owner {
		return errors.New("replacement placement ownership mismatch")
	}
	var placementID uint64
	if err := tx.Get(&placementID, "SELECT id FROM user_parents WHERE user_id=? AND node IN ('deactive','active') ORDER BY id LIMIT 1 FOR UPDATE", owner); err != nil {
		return err
	}
	var expiredOwner uint64
	if err := tx.Get(&expiredOwner, "SELECT user_id FROM registration_expiries WHERE placement_log_id=?", row.Old); err != nil {
		return err
	}
	if expiredOwner != owner {
		return errors.New("expired placement ownership mismatch")
	}
	if d.Blockchain == nil {
		return errors.New("replacement custody unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := d.Blockchain.ReopenRegistration(ctx, owner, row.Old, row.New, intentID); err != nil {
		return err
	}
	result, err := tx.Exec("UPDATE package_purchase_intents SET status='awaiting_deposit' WHERE id=? AND user_id=? AND status='awaiting_reopening'", intentID, owner)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	result, err = tx.Exec("UPDATE registration_reopening_outbox SET status='confirmed',confirmed_at=NOW(),last_error=NULL WHERE purchase_intent_id=? AND status='pending'", intentID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return tx.Commit()
}

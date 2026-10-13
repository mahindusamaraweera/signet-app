package handlers

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"signet-backend/internal/auth"
)

var errResetProof = errors.New("invalid or expired password reset proof")

func completePasswordReset(ctx context.Context, db *sql.DB, email, token, password string, now time.Time) error {
	if email == "" || token == "" || len(token) > 256 || len(password) < 8 || len(password) > 72 {
		return errResetProof
	}
	lookup, err := passwordResetLookup(token)
	if err != nil {
		return err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email=? FOR UPDATE`, email).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errResetProof
		}
		return err
	}
	var created time.Time
	if err := tx.QueryRowContext(ctx, `SELECT created_at FROM password_reset WHERE email=? AND token=? AND delivery_status IN ('legacy','delivered') ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, email, lookup).Scan(&created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errResetProof
		}
		return err
	}
	if created.After(now) || !now.Before(created.Add(time.Hour)) {
		return errResetProof
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_profiles (user_id) VALUES (?) ON DUPLICATE KEY UPDATE user_id=user_id`, userID); err != nil {
		return err
	}
	var version uint64
	var hold sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT security_version,hold_until FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&version, &hold); err != nil {
		return err
	}
	until := now.UTC().Add(48 * time.Hour)
	if hold.Valid && hold.Time.After(until) {
		until = hold.Time
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password=?,updated_at=? WHERE id=?`, hashed, now.UTC(), userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_profiles SET security_version=security_version+1,hold_until=? WHERE user_id=? AND security_version=?`, until, userID, version)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return errResetProof
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='superseded' WHERE user_id=? AND status IN ('pending_delivery','delivered')`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET status='invalidated' WHERE user_id=? AND status IN ('pending','authorized')`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset WHERE email=?`, email); err != nil {
		return err
	}
	reference := sha256.Sum256([]byte("password-reset-audit-v1|" + email + "|" + token))
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_audit (user_id,event,reference_id,security_version,created_at) VALUES (?,'password_reset',?,?,?)`, userID, hex.EncodeToString(reference[:]), version+1, now.UTC()); err != nil {
		return err
	}
	return tx.Commit()
}

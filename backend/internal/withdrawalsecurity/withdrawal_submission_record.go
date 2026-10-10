package withdrawalsecurity

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// Record only a validated remote reservation. Security changes do not erase
// already-reserved custody liabilities; cancellation/revalidation is separate.
func (s *Service) RecordWithdrawalReservation(ctx context.Context, id string, expected WithdrawalTerms, remoteID uint64, now time.Time) error {
	if remoteID == 0 || !challengeIDPattern.MatchString(id) {
		return ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var owner uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, expected.UserID).Scan(&owner); err != nil {
		return err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, expected.UserID, id)
	if err != nil || owner != expected.UserID || a.Terms != expected || a.Status != "consumed" {
		return ErrWithdrawalAuthorization
	}
	var encoded, digest, status string
	var storedRemote sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT terms,terms_digest,status,remote_withdrawal_id FROM withdrawal_submission_outbox WHERE authorization_id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&encoded, &digest, &status, &storedRemote); err != nil {
		return err
	}
	var stored WithdrawalTerms
	if json.Unmarshal([]byte(encoded), &stored) != nil || stored != expected || digest != expected.Digest() {
		return ErrWithdrawalAuthorization
	}
	if (status == "submitted" || status == "cancel_requested") && storedRemote.Valid && storedRemote.Int64 > 0 && uint64(storedRemote.Int64) == remoteID {
		return tx.Commit()
	}
	if (status != "pending" && status != "cancel_requested") || storedRemote.Valid {
		return ErrWithdrawalAuthorization
	}
	nextStatus := status
	if status == "pending" {
		nextStatus = "submitted"
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_submission_outbox SET status=?,remote_withdrawal_id=?,last_error=NULL,updated_at=? WHERE authorization_id=? AND user_id=? AND status=? AND remote_withdrawal_id IS NULL`, nextStatus, remoteID, now.UTC(), id, expected.UserID, status)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrWithdrawalAuthorization
	}
	result, err = tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET remote_withdrawal_id=? WHERE id=? AND user_id=? AND status='consumed' AND remote_withdrawal_id IS NULL`, remoteID, id, expected.UserID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrWithdrawalAuthorization
	}
	if err := recordAudit(ctx, tx, expected.UserID, expected.SecurityVersion, "withdrawal_submitted", id, now); err != nil {
		return err
	}
	return tx.Commit()
}

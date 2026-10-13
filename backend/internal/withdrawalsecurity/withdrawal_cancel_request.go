package withdrawalsecurity

import (
	"context"
	"encoding/json"
	"signet-backend/internal/auth"
	"time"
)

// Password-authenticated owner intent. Frozen/changed security does not prevent
// cancelling liabilities; no new payout authority is granted by cancellation.
func (s *Service) RequestWithdrawalCancellation(ctx context.Context, userID uint64, password, id string, now time.Time) error {
	if userID == 0 || password == "" || !challengeIDPattern.MatchString(id) {
		return ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hash string
	if err := tx.QueryRowContext(ctx, `SELECT password FROM users WHERE id=? FOR UPDATE`, userID).Scan(&hash); err != nil {
		return err
	}
	if !auth.CheckPassword(hash, password) {
		return ErrSecurityCredentials
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	if a.Status == "pending" || a.Status == "authorized" {
		result, err := tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET status='invalidated' WHERE id=? AND user_id=? AND status IN ('pending','authorized')`, id, userID)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return ErrWithdrawalAuthorization
		}
		if err := recordAudit(ctx, tx, userID, a.Terms.SecurityVersion, "withdrawal_cancelled_local", id, now); err != nil {
			return err
		}
		return tx.Commit()
	}
	if a.Status != "consumed" {
		return ErrWithdrawalAuthorization
	}
	var encoded, digest, status string
	if err := tx.QueryRowContext(ctx, `SELECT terms,terms_digest,status FROM withdrawal_submission_outbox WHERE authorization_id=? AND user_id=? FOR UPDATE`, id, userID).Scan(&encoded, &digest, &status); err != nil {
		return err
	}
	var stored WithdrawalTerms
	if json.Unmarshal([]byte(encoded), &stored) != nil || stored != a.Terms || digest != a.Terms.Digest() {
		return ErrWithdrawalAuthorization
	}
	if status == "cancel_requested" || status == "cancelled" {
		return tx.Commit()
	}
	if status != "pending" && status != "submitted" {
		return ErrWithdrawalAuthorization
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_submission_outbox SET status='cancel_requested',last_error=NULL,updated_at=? WHERE authorization_id=? AND user_id=? AND status=?`, now.UTC(), id, userID, status)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrWithdrawalAuthorization
	}
	if err := recordAudit(ctx, tx, userID, a.Terms.SecurityVersion, "withdrawal_cancel_requested", id, now); err != nil {
		return err
	}
	return tx.Commit()
}

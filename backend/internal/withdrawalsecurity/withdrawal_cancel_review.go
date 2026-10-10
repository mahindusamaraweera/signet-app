package withdrawalsecurity

import (
	"context"
	"time"
)

// A definite custody safety rejection is not a refund or retryable success.
// Preserve financial identity and stop automatic retries pending reconciliation.
func (s *Service) RecordWithdrawalCancellationReview(ctx context.Context, id string, expected WithdrawalTerms, now time.Time) error {
	if !challengeIDPattern.MatchString(id) {
		return ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, status, _, err := s.lockCancellationIntent(ctx, tx, id, expected)
	if err != nil {
		return err
	}
	if status != "cancel_requested" {
		return ErrWithdrawalAuthorization
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_submission_outbox SET status='cancellation_review_required',last_error='Cancellation cannot be confirmed safely; reconciliation required',updated_at=? WHERE authorization_id=? AND user_id=? AND status='cancel_requested'`, now.UTC(), id, expected.UserID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrWithdrawalAuthorization
	}
	if err := recordAudit(ctx, tx, expected.UserID, expected.SecurityVersion, "withdrawal_cancel_review", id, now); err != nil {
		return err
	}
	return tx.Commit()
}

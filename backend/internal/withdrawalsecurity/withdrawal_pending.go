package withdrawalsecurity

import (
	"context"
	"database/sql"
)

// Called while the owner row is locked: concurrent tabs cannot enqueue two
// unreserved requests before custody has had a chance to see either one.
func assertNoPendingWithdrawal(ctx context.Context, tx *sql.Tx, userID uint64, except string) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_submission_outbox WHERE user_id=? AND authorization_id<>? AND status IN ('pending','cancel_requested','cancellation_review_required')`, userID, except).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return eligibility("withdrawal_already_processing", "An existing withdrawal is processing or needs review. Wait for completion or confirmed cancellation before starting another.", ErrWithdrawalAuthorization)
	}
	return nil
}

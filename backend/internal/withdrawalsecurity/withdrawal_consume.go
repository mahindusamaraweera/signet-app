package withdrawalsecurity

import (
	"context"
	"database/sql"
	"time"
)

// Internal transaction step, not a public submission service. Caller must lock
// and revalidate current owner/profile/enrollment/address BEFORE loading a.
// The same transaction must create the durable submission outbox; token still
// reserves funds and runs its financial audit before broadcasting.
func consumeWithdrawalAuthorization(ctx context.Context, tx *sql.Tx, a WithdrawalAuthorization, key []byte, proof string, currentTerms WithdrawalTerms, now time.Time) error {
	if tx == nil || !a.CanConsume(key, proof, currentTerms, now) {
		return ErrWithdrawalAuthorization
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET status='consumed',consumed_at=? WHERE id=? AND user_id=? AND security_version=? AND terms_digest=? AND proof_hash=? AND status='authorized' AND authorized_at IS NOT NULL AND consumed_at IS NULL AND expires_at>?`, now.UTC(), a.ID, currentTerms.UserID, currentTerms.SecurityVersion, currentTerms.Digest(), a.ProofHash, now.UTC())
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrWithdrawalAuthorization
	}
	return recordAudit(ctx, tx, currentTerms.UserID, currentTerms.SecurityVersion, "withdrawal_consumed", a.ID, now)
}

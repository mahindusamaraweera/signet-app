package withdrawalsecurity

import (
	"context"
	"crypto/hmac"
	"database/sql"
	"errors"
	"time"

	"signet-backend/internal/totp"
)

// ConfirmWithdrawal grants short-lived source approval only, not a payout.
// Submission must consume it atomically and retain token's financial audit.
func (s *Service) ConfirmWithdrawal(ctx context.Context, userID uint64, password, id, proof string, expected WithdrawalTerms, challengeID, emailCode, authenticatorCode string, now time.Time) error {
	if userID == 0 || expected.UserID != userID || !challengeIDPattern.MatchString(id) || !challengeIDPattern.MatchString(proof) || !challengeIDPattern.MatchString(challengeID) {
		return ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	email, secret, err := s.lockWithdrawalState(ctx, tx, expected, password, now)
	if err != nil {
		return err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	if a.Terms != expected || a.Status != "pending" || !now.Before(a.ExpiresAt) || !hmac.Equal([]byte(a.ProofHash), []byte(a.hash(s.key, proof))) {
		return ErrWithdrawalAuthorization
	}
	var c Challenge
	var consumed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT id,user_id,purpose,target,email,code_hash,security_version,status,attempts,expires_at,consumed_at FROM withdrawal_security_challenges WHERE id=? AND user_id=? FOR UPDATE`, challengeID, userID).Scan(&c.ID, &c.UserID, &c.Purpose, &c.Target, &c.Email, &c.Hash, &c.SecurityVersion, &c.Status, &c.Attempts, &c.ExpiresAt, &consumed); err != nil {
		return err
	}
	if consumed.Valid {
		c.ConsumedAt = &consumed.Time
	}
	if c.Status != "delivered" || c.ConsumedAt != nil || c.Attempts >= 5 {
		return ErrInvalidChallenge
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET attempts=attempts+1 WHERE id=? AND user_id=?`, c.ID, userID); err != nil {
		return err
	}
	if !c.AcceptsWithdrawal(s.key, email, emailCode, a, now) {
		if err := tx.Commit(); err != nil {
			return err
		}
		return ErrInvalidChallenge
	}
	if err := totp.ConsumeWithdrawalCode(ctx, tx, userID, secret, authenticatorCode, now); err != nil {
		if errors.Is(err, totp.ErrInvalidOrReusedCode) {
			if err := tx.Commit(); err != nil {
				return err
			}
		}
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='consumed',consumed_at=? WHERE id=? AND user_id=? AND status='delivered' AND consumed_at IS NULL`, now.UTC(), c.ID, userID)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrInvalidChallenge
	}
	result, err = tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET status='authorized',authorized_at=? WHERE id=? AND user_id=? AND security_version=? AND terms_digest=? AND status='pending' AND authorized_at IS NULL AND consumed_at IS NULL AND expires_at>?`, now.UTC(), a.ID, userID, expected.SecurityVersion, expected.Digest(), now.UTC())
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrWithdrawalAuthorization
	}
	if err := recordAudit(ctx, tx, userID, expected.SecurityVersion, "withdrawal_authorized", a.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

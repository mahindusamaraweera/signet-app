package withdrawalsecurity

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"time"
)

var ErrWithdrawalEmailDelivery = errors.New("withdrawal email delivery failed")

func (s *Service) IssueWithdrawalEmail(ctx context.Context, userID uint64, password, id, proof string, expected WithdrawalTerms, now time.Time, mailer Mailer) (string, error) {
	if userID == 0 || expected.UserID != userID || !challengeIDPattern.MatchString(id) || !challengeIDPattern.MatchString(proof) || mailer == nil {
		return "", ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	email, _, err := s.lockWithdrawalState(ctx, tx, expected, password, now)
	if err != nil {
		return "", err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, userID, id)
	if err != nil {
		return "", err
	}
	if a.Terms != expected || a.Status != "pending" || !hmac.Equal([]byte(a.ProofHash), []byte(a.hash(s.key, proof))) {
		return "", ErrWithdrawalAuthorization
	}
	if !now.Before(a.ExpiresAt) {
		return "", ErrWithdrawalExpired
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, WithdrawalPurpose, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, WithdrawalPurpose, now.UTC().Add(-time.Hour)).Scan(&hourly); err != nil {
		return "", err
	}
	if recent > 0 || hourly >= 5 {
		return "", ErrChallengeCooldown
	}
	c, code, err := NewWithdrawalChallenge(s.key, email, a, now)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='superseded' WHERE user_id=? AND purpose=? AND status IN ('pending_delivery','delivered')`, userID, WithdrawalPurpose); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_challenges (id,user_id,purpose,target,email,code_hash,security_version,status,expires_at,created_at) VALUES (?,?,?,?,?,?,?,'pending_delivery',?,?)`, c.ID, userID, c.Purpose, c.Target, email, c.Hash, expected.SecurityVersion, c.ExpiresAt, now.UTC()); err != nil {
		return "", err
	}
	if err := recordAudit(ctx, tx, userID, expected.SecurityVersion, "withdrawal_email_requested", c.ID, now); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	body := fmt.Sprintf("Confirm SIGNET withdrawal on BSC\nDestination: %s\nGross SIGNET: %s\nFee SIGNET (deducted 3%%): %s\nUser receives SIGNET: %s\nVerification code: %s\nExpires at %s. Do not share this code. Ignore this message if you did not request this exact withdrawal.", expected.Destination, expected.Gross, expected.Fee, expected.Net, code, c.ExpiresAt.Format(time.RFC3339))
	deliveryErr := mailer.Send(ctx, email, "Confirm SIGNET withdrawal", body)
	status := "delivered"
	if deliveryErr != nil {
		status = "delivery_failed"
	}
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(recordCtx, `UPDATE withdrawal_security_challenges SET status=? WHERE id=? AND user_id=? AND status='pending_delivery'`, status, c.ID, userID)
	if err != nil {
		return "", err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return "", err
	} else if rows != 1 {
		return "", ErrInvalidChallenge
	}
	if deliveryErr != nil {
		return "", ErrWithdrawalEmailDelivery
	}
	return c.ID, nil
}

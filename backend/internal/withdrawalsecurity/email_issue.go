package withdrawalsecurity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"signet-backend/internal/auth"
)

type Mailer interface {
	Send(context.Context, string, string, string) error
}

var ErrChallengeCooldown = errors.New("email verification resend limit reached")
var ErrSecurityCredentials = errors.New("current password is required for security setup")

// IssueEmail returns only the challenge identity, never its secret code.
// The caller must apply authenticated per-user and per-IP request limits.
func (s *Service) IssueEmail(ctx context.Context, userID uint64, password string, now time.Time, mailer Mailer) (string, error) {
	if userID == 0 || password == "" || mailer == nil {
		return "", ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var email, passwordHash string
	if err := tx.QueryRowContext(ctx, `SELECT email,password FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &passwordHash); err != nil {
		return "", err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return "", ErrSecurityCredentials
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_profiles (user_id) VALUES (?) ON DUPLICATE KEY UPDATE user_id=user_id`, userID); err != nil {
		return "", err
	}
	var version uint64
	var frozen bool
	if err := tx.QueryRowContext(ctx, `SELECT security_version,withdrawals_frozen FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&version, &frozen); err != nil {
		return "", err
	}
	if frozen {
		return "", ErrSecurityCredentials
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, EmailPurpose, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, EmailPurpose, now.UTC().Add(-time.Hour)).Scan(&hourly); err != nil {
		return "", err
	}
	if recent > 0 || hourly >= 5 {
		return "", ErrChallengeCooldown
	}
	challenge, code, err := NewEmailChallenge(s.key, userID, version, email, now)
	if err != nil {
		return "", err
	}
	// A resend invalidates prior challenges before any new mail is delivered.
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='superseded' WHERE user_id=? AND purpose=? AND status IN ('pending_delivery','delivered')`, userID, EmailPurpose); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_challenges (id,user_id,purpose,email,code_hash,security_version,status,expires_at,created_at)
        VALUES (?,?,?,?,?,?,'pending_delivery',?,?)`, challenge.ID, userID, challenge.Purpose, email, challenge.Hash, version, challenge.ExpiresAt, now.UTC()); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	// Do not keep user/profile locks while communicating with SMTP.
	deliveryErr := mailer.Send(ctx, email, "Verify your withdrawal security email", fmt.Sprintf("Your verification code is %s. It expires in 10 minutes. Do not share this code. If you did not request it, ignore this message.", code))
	status := "delivered"
	if deliveryErr != nil {
		status = "delivery_failed"
	}
	// Request cancellation must not leave an accepted/failed delivery unrecorded.
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	result, err := s.db.ExecContext(recordCtx, `UPDATE withdrawal_security_challenges SET status=? WHERE id=? AND user_id=? AND status='pending_delivery'`, status, challenge.ID, userID)
	if err != nil {
		return "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows != 1 {
		return "", ErrInvalidChallenge
	}
	if deliveryErr != nil {
		return "", errors.New("verification email could not be delivered")
	}
	return challenge.ID, nil
}

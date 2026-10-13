package withdrawalsecurity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"signet-backend/internal/auth"
)

// IssueAddress requires current credentials and confirmed financial security.
// It only issues destination-bound proof; it does not whitelist the address.
func (s *Service) IssueAddress(ctx context.Context, userID uint64, password, address string, now time.Time, mailer Mailer) (string, error) {
	return s.issueAddressAction(ctx, userID, password, address, now, mailer, false)
}

func (s *Service) IssueAddressRevocation(ctx context.Context, userID uint64, password, addressID string, now time.Time, mailer Mailer) (string, error) {
	return s.issueAddressAction(ctx, userID, password, addressID, now, mailer, true)
}

func (s *Service) issueAddressAction(ctx context.Context, userID uint64, password, address string, now time.Time, mailer Mailer, revoke bool) (string, error) {
	if revoke {
		if !challengeIDPattern.MatchString(address) {
			return "", ErrWithdrawalAddress
		}
	} else if _, err := addressChallengeTarget(address); err != nil {
		return "", err
	}
	if userID == 0 || password == "" || mailer == nil {
		return "", ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var email, passwordHash string
	var legacy sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &passwordHash, &legacy); err != nil {
		return "", err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return "", ErrSecurityCredentials
	}
	var verified, fingerprint sql.NullString
	var verifiedAt, confirmed sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_fingerprint,authenticator_confirmed_at,withdrawals_frozen,security_version FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&verified, &verifiedAt, &fingerprint, &confirmed, &frozen, &version); err != nil {
		return "", err
	}
	if frozen || !verifiedAt.Valid || verified.String != email || !confirmed.Valid {
		return "", ErrSecurityCredentials
	}
	var encrypted, previous, enrolledEmail string
	var enrollmentVersion uint64
	var enrolledConfirmed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,confirmed_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).Scan(&encrypted, &previous, &enrolledEmail, &enrollmentVersion, &enrolledConfirmed); err != nil {
		return "", err
	}
	if !enrolledConfirmed.Valid || !confirmed.Time.Equal(enrolledConfirmed.Time) || enrolledEmail != email || enrollmentVersion == 0 || previous != secretFingerprint(s.key, legacy.String) {
		return "", ErrSecurityCredentials
	}
	secret, err := decryptEnrollment(s.key, userID, enrollmentVersion, email, encrypted)
	if err != nil {
		return "", err
	}
	if fingerprint.String != secretFingerprint(s.key, secret) {
		return "", ErrSecurityCredentials
	}
	purpose := AddressEnrollmentPurpose
	destination := address
	if revoke {
		purpose = AddressRevocationPurpose
		var revoked sql.NullTime
		if err := tx.QueryRowContext(ctx, `SELECT address,revoked_at FROM withdrawal_security_addresses WHERE id=? AND user_id=? AND network='BSC' AND asset='SIGNET' FOR UPDATE`, address, userID).Scan(&destination, &revoked); err != nil {
			return "", err
		}
		if revoked.Valid {
			return "", ErrWithdrawalAddress
		}
	}
	var recent, hourly int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, purpose, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return "", err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_challenges WHERE user_id=? AND purpose=? AND created_at>=?`, userID, purpose, now.UTC().Add(-time.Hour)).Scan(&hourly); err != nil {
		return "", err
	}
	if recent > 0 || hourly >= 5 {
		return "", ErrChallengeCooldown
	}
	c, code, err := NewAddressEnrollmentChallenge(s.key, userID, version, email, address, now)
	if revoke {
		c, code, err = NewAddressRevocationChallenge(s.key, userID, version, email, address, now)
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='superseded' WHERE user_id=? AND purpose=? AND status IN ('pending_delivery','delivered')`, userID, purpose); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_challenges (id,user_id,purpose,target,email,code_hash,security_version,status,expires_at,created_at) VALUES (?,?,?,?,?,?,?,'pending_delivery',?,?)`, c.ID, userID, c.Purpose, c.Target, email, c.Hash, version, c.ExpiresAt, now.UTC()); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	body := fmt.Sprintf("Your BSC withdrawal address: %s\nVerification code: %s\nExpires in 10 minutes. Confirmation requires your authenticator and starts a 24-hour address waiting period. Do not share this code. Ignore this message if you did not request it.", destination, code)
	if revoke {
		body = fmt.Sprintf("Revoke your SIGNET withdrawal address on BSC: %s\nVerification code: %s\nExpires in 10 minutes. Confirmation requires your authenticator and disables this destination. Do not share this code. Ignore this message if you did not request it.", destination, code)
	}
	deliveryErr := mailer.Send(ctx, email, "Verify your SIGNET withdrawal address", body)
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
	return c.ID, nil
}

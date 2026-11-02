package withdrawalsecurity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"signet-backend/internal/auth"
	"signet-backend/internal/totp"
)

func (s *Service) ConfirmAuthenticator(ctx context.Context, userID uint64, password, code string, now time.Time) error {
	if userID == 0 || password == "" {
		return ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var email, passwordHash string
	var previous sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &passwordHash, &previous); err != nil {
		return err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return ErrSecurityCredentials
	}
	var verifiedEmail sql.NullString
	var emailVerified, confirmed, hold sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_confirmed_at,withdrawals_frozen,security_version,hold_until FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).
		Scan(&verifiedEmail, &emailVerified, &confirmed, &frozen, &version, &hold); err != nil {
		return err
	}
	if frozen || confirmed.Valid || !emailVerified.Valid || verifiedEmail.String != email {
		return ErrAuthenticatorEnrollment
	}
	var encrypted, previousFingerprint, enrolledEmail string
	var enrollmentVersion uint64
	var attempts uint8
	var expires time.Time
	var enrollmentConfirmed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,attempts,expires_at,confirmed_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).
		Scan(&encrypted, &previousFingerprint, &enrolledEmail, &enrollmentVersion, &attempts, &expires, &enrollmentConfirmed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAuthenticatorEnrollment
	}
	if err != nil {
		return err
	}
	if attempts >= 5 || enrollmentConfirmed.Valid || !now.Before(expires) || enrollmentVersion != version || enrolledEmail != email || previousFingerprint != secretFingerprint(s.key, previous.String) {
		return ErrAuthenticatorEnrollment
	}
	secret, err := decryptEnrollment(s.key, userID, enrollmentVersion, enrolledEmail, encrypted)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_authenticator_enrollments SET attempts=attempts+1 WHERE user_id=?`, userID); err != nil {
		return err
	}
	err = totp.ConsumeWithdrawalCode(ctx, tx, userID, secret, code, now)
	if errors.Is(err, totp.ErrInvalidOrReusedCode) {
		if err := tx.Commit(); err != nil {
			return err
		}
		return totp.ErrInvalidOrReusedCode
	}
	if err != nil {
		return err
	}
	// Apply a 24-hour security-change hold, preserving longer existing holds.
	until := now.UTC().Add(24 * time.Hour)
	if hold.Valid && hold.Time.After(until) {
		until = hold.Time
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_profiles SET authenticator_fingerprint=?,authenticator_confirmed_at=?,security_version=security_version+1,hold_until=? WHERE user_id=? AND security_version=? AND authenticator_confirmed_at IS NULL`,
		secretFingerprint(s.key, secret), now.UTC(), until, userID, version)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAuthenticatorEnrollment
	}
	result, err = tx.ExecContext(ctx, `UPDATE withdrawal_authenticator_enrollments SET confirmed_at=? WHERE user_id=? AND confirmed_at IS NULL`, now.UTC(), userID)
	if err != nil {
		return err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrAuthenticatorEnrollment
	}
	// Keep the secret encrypted in the enrollment record. Do not overwrite
	// the legacy users authenticator column or its administrative workflows.
	if err := recordAudit(ctx, tx, userID, version+1, "authenticator_confirmed", securityChangeReference(userID, version+1, "authenticator_confirmed"), now); err != nil {
		return err
	}
	return tx.Commit()
}

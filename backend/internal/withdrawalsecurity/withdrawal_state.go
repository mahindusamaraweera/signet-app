package withdrawalsecurity

import (
	"context"
	"database/sql"
	"time"

	"signet-backend/internal/auth"
)

// Revalidate security under the common owner -> profile -> enrollment -> address
// lock order before touching an authorization. Never accept client identity or
// a prepared security version as proof of current account eligibility.
func (s *Service) lockWithdrawalState(ctx context.Context, tx *sql.Tx, terms WithdrawalTerms, password string, now time.Time) (string, string, error) {
	if password == "" {
		return "", "", ErrWithdrawalAuthorization
	}
	return s.lockWithdrawalCurrentState(ctx, tx, terms, &password, now)
}

// Nil password is private worker-only revalidation AFTER durable approval;
// callers must load consumed authorization and matching outbox before signing.
// Passwords must never be retained in a queue or sent to the custody service.
func (s *Service) lockWithdrawalCurrentState(ctx context.Context, tx *sql.Tx, terms WithdrawalTerms, password *string, now time.Time) (string, string, error) {
	canonical, err := NewWithdrawalTerms(terms.UserID, terms.SecurityVersion, terms.AddressID, terms.Destination, terms.Gross)
	if tx == nil || err != nil || canonical != terms {
		return "", "", ErrWithdrawalAuthorization
	}
	var email, passwordHash string
	var legacy sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, terms.UserID).Scan(&email, &passwordHash, &legacy); err != nil {
		return "", "", err
	}
	if password != nil && !auth.CheckPassword(passwordHash, *password) {
		return "", "", eligibility("withdrawal_password_incorrect", "The current password is incorrect. Enter your Signet account password.", ErrSecurityCredentials)
	}
	var verified, fingerprint sql.NullString
	var verifiedAt, confirmed, hold sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_fingerprint,authenticator_confirmed_at,withdrawals_frozen,security_version,hold_until FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, terms.UserID).Scan(&verified, &verifiedAt, &fingerprint, &confirmed, &frozen, &version, &hold); err != nil {
		if err == sql.ErrNoRows {
			return "", "", eligibility("withdrawal_security_setup_required", "Complete email, authenticator and address setup in Withdrawal security.", ErrSecurityCredentials)
		}
		return "", "", err
	}
	if err := withdrawalProfileEligibility(email, verified, verifiedAt, confirmed, hold, frozen, now); err != nil {
		return "", "", err
	}
	if version != terms.SecurityVersion {
		return "", "", eligibility("withdrawal_security_changed", "Your security settings changed. Start a new withdrawal after checking Withdrawal security.", ErrSecurityCredentials)
	}
	var encrypted, previous, enrolledEmail string
	var enrollmentVersion uint64
	var enrolledConfirmed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,confirmed_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, terms.UserID).Scan(&encrypted, &previous, &enrolledEmail, &enrollmentVersion, &enrolledConfirmed); err != nil {
		if err == sql.ErrNoRows {
			return "", "", eligibility("withdrawal_authenticator_setup_required", "Complete withdrawal authenticator setup in Withdrawal security.", ErrSecurityCredentials)
		}
		return "", "", err
	}
	if err := withdrawalEnrollmentEligibility(confirmed, enrolledConfirmed, email, enrolledEmail, enrollmentVersion, previous, secretFingerprint(s.key, legacy.String)); err != nil {
		return "", "", err
	}
	secret, err := decryptEnrollment(s.key, terms.UserID, enrollmentVersion, email, encrypted)
	if err != nil {
		return "", "", eligibility("withdrawal_authenticator_unavailable", "Your withdrawal authenticator could not be validated. Contact support to check the withdrawal security configuration.", ErrSecurityCredentials)
	}
	if fingerprint.String != secretFingerprint(s.key, secret) {
		return "", "", eligibility("withdrawal_authenticator_changed", "Your withdrawal authenticator setup does not match your confirmed settings. Contact support to review Withdrawal security.", ErrSecurityCredentials)
	}
	var destination string
	var eligible, verifiedAddress time.Time
	var revoked sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT address,verified_at,eligible_at,revoked_at FROM withdrawal_security_addresses WHERE id=? AND user_id=? AND network='BSC' AND asset='SIGNET' FOR UPDATE`, terms.AddressID, terms.UserID).Scan(&destination, &verifiedAddress, &eligible, &revoked); err != nil {
		if err == sql.ErrNoRows {
			return "", "", eligibility("withdrawal_address_missing", "This withdrawal address was not found. Refresh and select your verified address in Withdrawal security.", ErrSecurityCredentials)
		}
		return "", "", err
	}
	if err := withdrawalAddressEligibility(verifiedAddress, eligible, revoked, now); err != nil {
		return "", "", err
	}
	if destination != terms.Destination {
		return "", "", eligibility("withdrawal_address_changed", "The withdrawal address changed. Start a new withdrawal with your verified address.", ErrWithdrawalAddress)
	}
	return email, secret, nil
}

package withdrawalsecurity

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"signet-backend/internal/auth"
)

type PreparedWithdrawal struct {
	ID        string          `json:"id"`
	Proof     string          `json:"proof"`
	Terms     WithdrawalTerms `json:"terms"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// Preparation does not reserve funds or authorize payout. The client proof must
// be transient; only its keyed hash is persisted. No device gate is applied.
func (s *Service) PrepareWithdrawal(ctx context.Context, userID uint64, password, addressID, gross string, now time.Time) (PreparedWithdrawal, error) {
	if userID == 0 || password == "" || !challengeIDPattern.MatchString(addressID) {
		return PreparedWithdrawal{}, ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PreparedWithdrawal{}, err
	}
	defer tx.Rollback()
	var email, passwordHash string
	var legacy sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &passwordHash, &legacy); err != nil {
		return PreparedWithdrawal{}, err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return PreparedWithdrawal{}, eligibility("withdrawal_password_incorrect", "The current password is incorrect. Enter your Signet account password.", ErrSecurityCredentials)
	}
	var verified, fingerprint sql.NullString
	var verifiedAt, confirmed, hold sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_fingerprint,authenticator_confirmed_at,withdrawals_frozen,security_version,hold_until FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&verified, &verifiedAt, &fingerprint, &confirmed, &frozen, &version, &hold); err != nil {
		if err == sql.ErrNoRows {
			return PreparedWithdrawal{}, eligibility("withdrawal_security_setup_required", "Complete email, authenticator and address setup in Withdrawal security.", ErrSecurityCredentials)
		}
		return PreparedWithdrawal{}, err
	}
	if err := withdrawalProfileEligibility(email, verified, verifiedAt, confirmed, hold, frozen, now); err != nil {
		return PreparedWithdrawal{}, err
	}
	var encrypted, previous, enrolledEmail string
	var enrollmentVersion uint64
	var enrolledConfirmed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,confirmed_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).Scan(&encrypted, &previous, &enrolledEmail, &enrollmentVersion, &enrolledConfirmed); err != nil {
		if err == sql.ErrNoRows {
			return PreparedWithdrawal{}, eligibility("withdrawal_authenticator_setup_required", "Complete withdrawal authenticator setup in Withdrawal security.", ErrSecurityCredentials)
		}
		return PreparedWithdrawal{}, err
	}
	if err := withdrawalEnrollmentEligibility(confirmed, enrolledConfirmed, email, enrolledEmail, enrollmentVersion, previous, secretFingerprint(s.key, legacy.String)); err != nil {
		return PreparedWithdrawal{}, err
	}
	secret, err := decryptEnrollment(s.key, userID, enrollmentVersion, email, encrypted)
	if err != nil {
		return PreparedWithdrawal{}, eligibility("withdrawal_authenticator_unavailable", "Your withdrawal authenticator could not be validated. Contact support to check the withdrawal security configuration.", ErrSecurityCredentials)
	}
	if fingerprint.String != secretFingerprint(s.key, secret) {
		return PreparedWithdrawal{}, eligibility("withdrawal_authenticator_changed", "Your withdrawal authenticator setup does not match your confirmed settings. Contact support to review Withdrawal security.", ErrSecurityCredentials)
	}
	var destination string
	var eligible, verifiedAddress time.Time
	var revoked sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT address,verified_at,eligible_at,revoked_at FROM withdrawal_security_addresses WHERE id=? AND user_id=? AND network='BSC' AND asset='SIGNET' FOR UPDATE`, addressID, userID).Scan(&destination, &verifiedAddress, &eligible, &revoked); err != nil {
		if err == sql.ErrNoRows {
			return PreparedWithdrawal{}, eligibility("withdrawal_address_missing", "This withdrawal address was not found. Refresh and select your verified address in Withdrawal security.", ErrSecurityCredentials)
		}
		return PreparedWithdrawal{}, err
	}
	if err := withdrawalAddressEligibility(verifiedAddress, eligible, revoked, now); err != nil {
		return PreparedWithdrawal{}, err
	}
	if err := assertNoPendingWithdrawal(ctx, tx, userID, ""); err != nil {
		return PreparedWithdrawal{}, err
	}
	var recent int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_authorizations WHERE user_id=? AND created_at>=?`, userID, now.UTC().Add(-time.Minute)).Scan(&recent); err != nil {
		return PreparedWithdrawal{}, err
	}
	if recent >= 5 {
		return PreparedWithdrawal{}, ErrChallengeCooldown
	}
	terms, err := NewWithdrawalTerms(userID, version, addressID, destination, gross)
	if err != nil {
		return PreparedWithdrawal{}, err
	}
	a, proof, err := NewWithdrawalAuthorization(s.key, terms, now)
	if err != nil {
		return PreparedWithdrawal{}, err
	}
	encoded, err := json.Marshal(terms)
	if err != nil {
		return PreparedWithdrawal{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_authorizations (id,user_id,security_version,address_id,terms,terms_digest,proof_hash,status,expires_at,created_at) VALUES (?,?,?,?,?,?,?,'pending',?,?)`, a.ID, userID, version, addressID, string(encoded), terms.Digest(), a.ProofHash, a.ExpiresAt, now.UTC()); err != nil {
		return PreparedWithdrawal{}, err
	}
	if err := recordAudit(ctx, tx, userID, version, "withdrawal_prepared", a.ID, now); err != nil {
		return PreparedWithdrawal{}, err
	}
	if err := tx.Commit(); err != nil {
		return PreparedWithdrawal{}, err
	}
	return PreparedWithdrawal{ID: a.ID, Proof: proof, Terms: terms, ExpiresAt: a.ExpiresAt}, nil
}

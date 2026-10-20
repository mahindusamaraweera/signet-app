package withdrawalsecurity

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"signet-backend/internal/auth"
	"signet-backend/internal/totp"
)

type AuthenticatorSetup struct {
	Existing  bool      `json:"existing_authenticator"`
	Secret    string    `json:"secret,omitempty"`
	URL       string    `json:"otpauth_url,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

var ErrAuthenticatorEnrollment = errors.New("authenticator enrollment is not eligible")

func (s *Service) BeginAuthenticator(ctx context.Context, userID uint64, password, emailChallengeID, emailCode string, now time.Time) (AuthenticatorSetup, error) {
	if userID == 0 || password == "" {
		return AuthenticatorSetup{}, ErrSecurityCredentials
	}
	var passwordHash string
	if err := s.db.QueryRowContext(ctx, `SELECT password FROM users WHERE id=?`, userID).Scan(&passwordHash); err != nil {
		return AuthenticatorSetup{}, err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return AuthenticatorSetup{}, ErrSecurityCredentials
	}
	// Fresh mailbox possession is required, not merely a historical timestamp.
	if err := s.VerifyEmail(ctx, userID, emailChallengeID, emailCode, now); err != nil {
		return AuthenticatorSetup{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AuthenticatorSetup{}, err
	}
	defer tx.Rollback()
	var email, lockedPassword string
	var previous sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &lockedPassword, &previous); err != nil {
		return AuthenticatorSetup{}, err
	}
	if !auth.CheckPassword(lockedPassword, password) {
		return AuthenticatorSetup{}, ErrSecurityCredentials
	}
	var verifiedEmail sql.NullString
	var emailVerified, confirmed sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_confirmed_at,withdrawals_frozen,security_version FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).
		Scan(&verifiedEmail, &emailVerified, &confirmed, &frozen, &version); err != nil {
		return AuthenticatorSetup{}, err
	}
	if frozen || confirmed.Valid || !emailVerified.Valid || verifiedEmail.String != email {
		return AuthenticatorSetup{}, ErrAuthenticatorEnrollment
	}
	var pending pendingAuthenticatorEnrollment
	err = tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,attempts,expires_at,confirmed_at,created_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).
		Scan(&pending.encrypted, &pending.previousFingerprint, &pending.email, &pending.version, &pending.attempts, &pending.expires, &pending.confirmed, &pending.created)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AuthenticatorSetup{}, err
	}
	if err == nil && now.Before(pending.created.Add(time.Minute)) {
		return AuthenticatorSetup{}, ErrChallengeCooldown
	}
	secret := previous.String
	existing := secret != ""
	expires := now.UTC().Add(10 * time.Minute)
	created := now.UTC()
	attempts := uint8(0)
	resumed, reusable, resumeErr := s.reusableAuthenticatorSecret(userID, version, email, previous.String, now, pending)
	if resumeErr != nil {
		return AuthenticatorSetup{}, resumeErr
	}
	if reusable {
		secret = resumed
		expires = pending.expires
		created = pending.created
		attempts = pending.attempts
	} else if !existing {
		secret, err = newAuthenticatorSecret()
		if err != nil {
			return AuthenticatorSetup{}, err
		}
	}
	encrypted, err := encryptEnrollment(s.key, userID, version, email, secret)
	if err != nil {
		return AuthenticatorSetup{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_authenticator_enrollments
        (user_id,encrypted_secret,previous_fingerprint,email,security_version,attempts,expires_at,confirmed_at,created_at)
        VALUES (?,?,?,?,?,?,?,NULL,?) ON DUPLICATE KEY UPDATE encrypted_secret=VALUES(encrypted_secret),previous_fingerprint=VALUES(previous_fingerprint),email=VALUES(email),security_version=VALUES(security_version),attempts=VALUES(attempts),expires_at=VALUES(expires_at),confirmed_at=NULL,created_at=VALUES(created_at)`,
		userID, encrypted, secretFingerprint(s.key, previous.String), email, version, attempts, expires, created); err != nil {
		return AuthenticatorSetup{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthenticatorSetup{}, err
	}
	result := AuthenticatorSetup{Existing: existing, ExpiresAt: expires}
	if !existing {
		result.Secret = secret
		result.URL = totp.OtpauthURL("signetint", email, secret)
	}
	return result, nil
}

type pendingAuthenticatorEnrollment struct {
	encrypted, previousFingerprint, email string
	version                               uint64
	attempts                              uint8
	expires, created                      time.Time
	confirmed                             sql.NullTime
}

// VerifyEmail immediately before this operation increments the security version
// once. Resume only that exact prior version, and retain its expiry and failed
// attempts. Other security changes, expired setups or exhausted attempts require
// a fresh setup. Never silently invalidate a still-valid QR on a normal retry.
func (s *Service) reusableAuthenticatorSecret(userID, version uint64, email, legacy string, now time.Time, pending pendingAuthenticatorEnrollment) (string, bool, error) {
	if pending.encrypted == "" || pending.confirmed.Valid || pending.attempts >= 5 || !now.Before(pending.expires) || version == 0 || pending.version != version-1 || pending.email != email || pending.previousFingerprint != secretFingerprint(s.key, legacy) {
		return "", false, nil
	}
	secret, err := decryptEnrollment(s.key, userID, pending.version, email, pending.encrypted)
	return secret, err == nil, err
}

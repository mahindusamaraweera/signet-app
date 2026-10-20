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
	var created time.Time
	err = tx.QueryRowContext(ctx, `SELECT created_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).Scan(&created)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return AuthenticatorSetup{}, err
	}
	if err == nil && now.Before(created.Add(time.Minute)) {
		return AuthenticatorSetup{}, ErrChallengeCooldown
	}
	secret := previous.String
	existing := secret != ""
	if !existing {
		secret, err = newAuthenticatorSecret()
		if err != nil {
			return AuthenticatorSetup{}, err
		}
	}
	encrypted, err := encryptEnrollment(s.key, userID, version, email, secret)
	if err != nil {
		return AuthenticatorSetup{}, err
	}
	expires := now.UTC().Add(10 * time.Minute)
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_authenticator_enrollments
        (user_id,encrypted_secret,previous_fingerprint,email,security_version,attempts,expires_at,confirmed_at,created_at)
        VALUES (?,?,?,?,?,0,?,NULL,?) ON DUPLICATE KEY UPDATE encrypted_secret=VALUES(encrypted_secret),previous_fingerprint=VALUES(previous_fingerprint),email=VALUES(email),security_version=VALUES(security_version),attempts=0,expires_at=VALUES(expires_at),confirmed_at=NULL,created_at=VALUES(created_at)`,
		userID, encrypted, secretFingerprint(s.key, previous.String), email, version, expires, now.UTC()); err != nil {
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

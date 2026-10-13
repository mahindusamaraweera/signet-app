package withdrawalsecurity

import (
	"context"
	"database/sql"
	"time"
)

type Status struct {
	EmailVerified          bool       `json:"email_verified"`
	AuthenticatorConfirmed bool       `json:"authenticator_confirmed"`
	Frozen                 bool       `json:"withdrawals_frozen"`
	HoldUntil              *time.Time `json:"hold_until"`
	SubmissionEnabled      bool       `json:"withdrawal_submission_enabled"`
	CancellationEnabled    bool       `json:"withdrawal_cancellation_enabled"`
}

func (s *Service) Status(ctx context.Context, userID uint64) (Status, error) {
	var email string
	var legacy, verifiedEmail, fingerprint, encrypted, previous, enrolledEmail sql.NullString
	var verifiedAt, confirmedAt, hold, enrolledConfirmed sql.NullTime
	var version sql.NullInt64
	var frozen sql.NullBool
	err := s.db.QueryRowContext(ctx, `SELECT u.email,u.google_authenticator_secret,p.verified_email,p.email_verified_at,p.authenticator_fingerprint,p.authenticator_confirmed_at,p.withdrawals_frozen,p.hold_until,
        e.encrypted_secret,e.previous_fingerprint,e.email,e.security_version,e.confirmed_at
        FROM users u LEFT JOIN withdrawal_security_profiles p ON p.user_id=u.id LEFT JOIN withdrawal_authenticator_enrollments e ON e.user_id=u.id WHERE u.id=?`, userID).
		Scan(&email, &legacy, &verifiedEmail, &verifiedAt, &fingerprint, &confirmedAt, &frozen, &hold, &encrypted, &previous, &enrolledEmail, &version, &enrolledConfirmed)
	if err != nil {
		return Status{}, err
	}
	result := Status{EmailVerified: verifiedAt.Valid && verifiedEmail.String == email, Frozen: frozen.Bool}
	if hold.Valid {
		result.HoldUntil = &hold.Time
	}
	if result.EmailVerified && confirmedAt.Valid && enrolledConfirmed.Valid && confirmedAt.Time.Equal(enrolledConfirmed.Time) && encrypted.Valid && enrolledEmail.String == email && version.Valid && version.Int64 > 0 && previous.String == secretFingerprint(s.key, legacy.String) {
		secret, err := decryptEnrollment(s.key, userID, uint64(version.Int64), email, encrypted.String)
		if err != nil {
			return Status{}, err
		}
		result.AuthenticatorConfirmed = fingerprint.String == secretFingerprint(s.key, secret)
	}
	// Submission is intentionally unavailable until verified-address and token
	// authorization bridge are implemented and verified, not inferred here.
	// Device verification is deferred and is not a submission requirement.
	return result, nil
}

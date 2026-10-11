package withdrawalsecurity

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"signet-backend/internal/auth"
	"signet-backend/internal/totp"
)

func (s *Service) ConfirmAddress(ctx context.Context, userID uint64, password, address, challengeID, emailCode, authenticatorCode string, now time.Time) error {
	return s.confirmAddressAction(ctx, userID, password, address, challengeID, emailCode, authenticatorCode, now, false)
}

func (s *Service) ConfirmAddressRevocation(ctx context.Context, userID uint64, password, addressID, challengeID, emailCode, authenticatorCode string, now time.Time) error {
	return s.confirmAddressAction(ctx, userID, password, addressID, challengeID, emailCode, authenticatorCode, now, true)
}

func (s *Service) confirmAddressAction(ctx context.Context, userID uint64, password, address, challengeID, emailCode, authenticatorCode string, now time.Time, revoke bool) error {
	if userID == 0 || password == "" || !challengeIDPattern.MatchString(challengeID) {
		return ErrSecurityCredentials
	}
	if revoke {
		if !challengeIDPattern.MatchString(address) {
			return ErrWithdrawalAddress
		}
	} else if _, err := addressChallengeTarget(address); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var email, passwordHash string
	var legacy sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT email,password,google_authenticator_secret FROM users WHERE id=? FOR UPDATE`, userID).Scan(&email, &passwordHash, &legacy); err != nil {
		return err
	}
	if !auth.CheckPassword(passwordHash, password) {
		return ErrSecurityCredentials
	}
	var verified, fingerprint sql.NullString
	var verifiedAt, confirmed sql.NullTime
	var frozen bool
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT verified_email,email_verified_at,authenticator_fingerprint,authenticator_confirmed_at,withdrawals_frozen,security_version FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&verified, &verifiedAt, &fingerprint, &confirmed, &frozen, &version); err != nil {
		return err
	}
	if frozen || !verifiedAt.Valid || verified.String != email || !confirmed.Valid {
		return ErrSecurityCredentials
	}
	var encrypted, previous, enrolledEmail string
	var enrollmentVersion uint64
	var enrolledConfirmed sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT encrypted_secret,previous_fingerprint,email,security_version,confirmed_at FROM withdrawal_authenticator_enrollments WHERE user_id=? FOR UPDATE`, userID).Scan(&encrypted, &previous, &enrolledEmail, &enrollmentVersion, &enrolledConfirmed); err != nil {
		return err
	}
	if !enrolledConfirmed.Valid || !confirmed.Time.Equal(enrolledConfirmed.Time) || enrolledEmail != email || enrollmentVersion == 0 || previous != secretFingerprint(s.key, legacy.String) {
		return ErrSecurityCredentials
	}
	secret, err := decryptEnrollment(s.key, userID, enrollmentVersion, email, encrypted)
	if err != nil {
		return err
	}
	if fingerprint.String != secretFingerprint(s.key, secret) {
		return ErrSecurityCredentials
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
	accepted := c.AcceptsAddress(s.key, userID, version, email, address, emailCode, now)
	if revoke {
		accepted = c.AcceptsAddressRevocation(s.key, userID, version, email, address, emailCode, now)
	}
	if !accepted {
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
	// Never let re-enrollment silently restore a revoked address or reset an
	// existing waiting period. Support must resolve revoked destinations explicitly.
	event := "address_verified"
	if revoke {
		var revoked sql.NullTime
		if err := tx.QueryRowContext(ctx, `SELECT revoked_at FROM withdrawal_security_addresses WHERE id=? AND user_id=? AND network='BSC' AND asset='SIGNET' FOR UPDATE`, address, userID).Scan(&revoked); err != nil {
			return err
		}
		if revoked.Valid {
			return ErrWithdrawalAddress
		}
		result, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_addresses SET revoked_at=? WHERE id=? AND user_id=? AND revoked_at IS NULL`, now.UTC(), address, userID)
		if err != nil {
			return err
		}
		if rows, err := result.RowsAffected(); err != nil {
			return err
		} else if rows != 1 {
			return ErrWithdrawalAddress
		}
		event = "address_revoked"
	} else {
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT id FROM withdrawal_security_addresses WHERE user_id=? AND network='BSC' AND asset='SIGNET' AND address=? FOR UPDATE`, userID, strings.ToLower(address)).Scan(&existing)
		if err == nil {
			return ErrWithdrawalAddress
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		id := make([]byte, 32)
		if _, err := rand.Read(id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_addresses (id,user_id,network,asset,address,security_version,verified_at,eligible_at,created_at) VALUES (?,?,'BSC','SIGNET',?,?,?,?,?)`, hex.EncodeToString(id), userID, strings.ToLower(address), version+1, now.UTC(), addressEligibleAt(now), now.UTC()); err != nil {
			return err
		}
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
	result, err = tx.ExecContext(ctx, `UPDATE withdrawal_security_profiles SET security_version=security_version+1 WHERE user_id=? AND security_version=?`, userID, version)
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows != 1 {
		return ErrInvalidChallenge
	}
	if err := recordAudit(ctx, tx, userID, version+1, event, c.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

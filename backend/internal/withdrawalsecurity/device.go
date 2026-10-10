package withdrawalsecurity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Device struct {
	ID         string     `json:"id"`
	VerifiedAt *time.Time `json:"verified_at"`
	EligibleAt *time.Time `json:"eligible_at"`
	Revoked    bool       `json:"revoked"`
}

func randomDeviceValue() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (s *Service) deviceCredentialHash(userID uint64, token string) string {
	mac := hmac.New(sha256.New, s.key)
	fmt.Fprintf(mac, "withdrawal-device-v1|%d|%s", userID, token)
	return hex.EncodeToString(mac.Sum(nil))
}

// RecordDevice only identifies a browser; it never grants trust or eligibility.
// The returned credential belongs in an HttpOnly cookie, not JSON or storage.
func (s *Service) RecordDevice(ctx context.Context, userID uint64, credential string, now time.Time) (Device, string, error) {
	if userID == 0 {
		return Device{}, "", ErrSecurityCredentials
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, "", err
	}
	defer tx.Rollback()
	var lockedID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, userID).Scan(&lockedID); err != nil {
		return Device{}, "", err
	}
	var result Device
	var verified, eligible, revoked sql.NullTime
	if challengeIDPattern.MatchString(credential) {
		err = tx.QueryRowContext(ctx, `SELECT id,verified_at,eligible_at,revoked_at FROM withdrawal_security_devices WHERE user_id=? AND credential_hash=? FOR UPDATE`, userID, s.deviceCredentialHash(userID, credential)).Scan(&result.ID, &verified, &eligible, &revoked)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Device{}, "", err
		}
	} else {
		err = sql.ErrNoRows
	}
	if errors.Is(err, sql.ErrNoRows) {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM withdrawal_security_devices WHERE user_id=?`, userID).Scan(&count); err != nil {
			return Device{}, "", err
		}
		if count >= 50 {
			return Device{}, "", errors.New("device record limit reached; contact support")
		}
		credential, err = randomDeviceValue()
		if err != nil {
			return Device{}, "", err
		}
		result.ID, err = randomDeviceValue()
		if err != nil {
			return Device{}, "", err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_devices (id,user_id,credential_hash,last_seen_at,created_at) VALUES (?,?,?,?,?)`, result.ID, userID, s.deviceCredentialHash(userID, credential), now.UTC(), now.UTC()); err != nil {
			return Device{}, "", err
		}
	} else {
		if verified.Valid {
			result.VerifiedAt = &verified.Time
		}
		if eligible.Valid {
			result.EligibleAt = &eligible.Time
		}
		result.Revoked = revoked.Valid
		if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_devices SET last_seen_at=? WHERE id=? AND user_id=?`, now.UTC(), result.ID, userID); err != nil {
			return Device{}, "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return Device{}, "", err
	}
	return result, credential, nil
}

package registrationverification

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"signet-backend/internal/auth"
)

var ErrVerification = errors.New("registration verification is invalid or expired")

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func randomReferral() (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	value := make([]byte, 6)
	for i := range value {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		value[i] = alphabet[n.Int64()]
	}
	return string(value), nil
}

func (s *Service) Complete(ctx context.Context, id, token string, now time.Time) (uint64, error) {
	if !tokenPattern.MatchString(id) || !tokenPattern.MatchString(token) {
		return 0, ErrVerification
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var lockID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM registration_completion_locks WHERE id=1 FOR UPDATE`).Scan(&lockID); err != nil {
		return 0, err
	}
	var email string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM pending_email_registrations WHERE id=?`, id).Scan(&email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrVerification
		}
		return 0, err
	}
	var guardedEmail string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM registration_email_guards WHERE email=? FOR UPDATE`, email).Scan(&guardedEmail); err != nil {
		return 0, err
	}
	var p Pending
	var verified sql.NullTime
	var payload, passwordHash string
	err = tx.QueryRowContext(ctx, `SELECT id,email,verification_hash,status,expires_at,verified_at,registration_data,password_hash FROM pending_email_registrations WHERE id=? FOR UPDATE`, id).
		Scan(&p.ID, &p.Email, &p.VerificationHash, &p.Status, &p.ExpiresAt, &verified, &payload, &passwordHash)
	if err != nil {
		return 0, err
	}
	if verified.Valid {
		p.VerifiedAt = &verified.Time
	}
	if p.Email != guardedEmail || !p.Accepts(s.key, token, now) {
		return 0, ErrVerification
	}
	var data Data
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		return 0, err
	}
	if data.Name == "" || data.Whatsapp == "" || data.CountryID == 0 || data.ReferralCode == "" || passwordHash == "" {
		return 0, ErrVerification
	}
	var duplicates int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE email=? OR whatsapp_number=? OR (?<>'' AND binance_pay_id=?)`, p.Email, data.Whatsapp, data.BinancePayID, data.BinancePayID).Scan(&duplicates); err != nil {
		return 0, err
	}
	if duplicates > 0 {
		return 0, ErrEmailExists
	}
	var sponsor uint64
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM referral_codes WHERE code=?`, data.ReferralCode).Scan(&sponsor); err != nil {
		return 0, ErrVerification
	}
	for _, candidate := range []string{data.LeaderCode, data.ExecutiveCode} {
		if candidate == "" {
			continue
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id=?`, candidate).Scan(&count); err != nil {
			return 0, err
		}
		if count != 1 {
			return 0, ErrVerification
		}
	}
	if data.LeaderCode != "" && data.LeaderCode == data.ExecutiveCode {
		return 0, ErrVerification
	}
	var country int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM countries WHERE id=?`, data.CountryID).Scan(&country); err != nil {
		return 0, err
	}
	if country != 1 {
		return 0, ErrVerification
	}
	userUUID, err := newUUID()
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO users (uuid,name,email,password,whatsapp_number,binance_pay_id,status,leader_code,executive_code,referred_by,country_id,email_verified_at,created_at,updated_at)
        VALUES (?,?,?,?,?,?,'pending',?,?,?,?,?,?,?)`, userUUID, data.Name, p.Email, passwordHash, data.Whatsapp, data.BinancePayID,
		strings.TrimLeft(data.LeaderCode, "0"), strings.TrimLeft(data.ExecutiveCode, "0"), sponsor, data.CountryID, now.UTC(), now.UTC(), now.UTC())
	if err != nil {
		return 0, err
	}
	userID, err := result.LastInsertId()
	if err != nil || userID <= 0 {
		return 0, errors.New("account identity unavailable")
	}
	// Deliberately enroll only brand-new verified registrations. Existing users
	// are never backfilled into automatic account deletion.
	if _, err := tx.ExecContext(ctx, `INSERT INTO registration_expiry_enrollments (user_id,policy_version,registered_at) VALUES (?,'new_unpaid_v1',?)`, userID, now.UTC()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO wallets (user_id,balance,created_at,updated_at) VALUES (?,0,?,?)`, userID, now.UTC(), now.UTC()); err != nil {
		return 0, err
	}
	referral, err := randomReferral()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO referral_codes (user_id,code,created_at,updated_at) VALUES (?,?,?,?)`, userID, strings.ToUpper(referral), now.UTC(), now.UTC()); err != nil {
		return 0, err
	}
	secret, err := randomHex(20)
	if err != nil {
		return 0, err
	}
	secretHash, err := auth.HashPassword(fmt.Sprintf("USER-%d-%s", userID, secret))
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_secret_keys (user_id,secret_key,created_at,updated_at) VALUES (?,?,?,?)`, userID, secretHash, now.UTC(), now.UTC()); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_security_profiles (user_id,verified_email,email_verified_at) VALUES (?,?,?)`, userID, p.Email, now.UTC()); err != nil {
		return 0, err
	}
	result, err = tx.ExecContext(ctx, `UPDATE pending_email_registrations SET status='verified',verified_at=?,user_id=? WHERE id=? AND status='delivered' AND verified_at IS NULL`, now.UTC(), userID, p.ID)
	if err != nil {
		return 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rows != 1 {
		return 0, ErrVerification
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint64(userID), nil
}

// newUUID returns a random RFC 4122 version-4 UUID for users.uuid.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

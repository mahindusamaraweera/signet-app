package withdrawalsecurity

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"
)

var ErrInvalidChallenge = errors.New("invalid, expired or exhausted email challenge")
var challengeIDPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Service struct {
	requireUUID bool
	db          *sql.DB
	key         []byte
}

func NewService(db *sql.DB, key []byte) (*Service, error) {
	if db == nil || len(key) < 32 {
		return nil, errors.New("security challenges require a database and strong signing key")
	}
	return &Service{db: db, key: append([]byte(nil), key...)}, nil
}

func (s *Service) VerifyEmail(ctx context.Context, userID uint64, challengeID, submitted string, now time.Time) error {
	if userID == 0 || !challengeIDPattern.MatchString(challengeID) {
		return ErrInvalidChallenge
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// All security operations use user -> profile -> challenge lock order.
	var currentEmail string
	if err := tx.QueryRowContext(ctx, `SELECT email FROM users WHERE id=? FOR UPDATE`, userID).Scan(&currentEmail); err != nil {
		return err
	}
	var version uint64
	var frozen bool
	if err := tx.QueryRowContext(ctx, `SELECT security_version, withdrawals_frozen FROM withdrawal_security_profiles WHERE user_id=? FOR UPDATE`, userID).Scan(&version, &frozen); err != nil {
		return err
	}
	if frozen {
		return ErrInvalidChallenge
	}
	var c Challenge
	var consumed sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT id,user_id,purpose,email,code_hash,security_version,status,attempts,expires_at,consumed_at
        FROM withdrawal_security_challenges WHERE id=? AND user_id=? FOR UPDATE`, challengeID, userID).
		Scan(&c.ID, &c.UserID, &c.Purpose, &c.Email, &c.Hash, &c.SecurityVersion, &c.Status, &c.Attempts, &c.ExpiresAt, &consumed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInvalidChallenge
	}
	if err != nil {
		return err
	}
	if consumed.Valid {
		c.ConsumedAt = &consumed.Time
	}
	if c.Status != "delivered" || c.ConsumedAt != nil || c.Attempts >= 5 {
		return ErrInvalidChallenge
	}
	accepted := c.Accepts(s.key, userID, version, currentEmail, submitted, now)
	if _, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET attempts=attempts+1 WHERE id=? AND user_id=?`, c.ID, userID); err != nil {
		return err
	}
	if !accepted {
		// Commit failed attempts; rolling back would allow unlimited guesses.
		if err := tx.Commit(); err != nil {
			return err
		}
		return ErrInvalidChallenge
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_security_challenges SET status='consumed',consumed_at=? WHERE id=? AND user_id=? AND status='delivered' AND consumed_at IS NULL`, now.UTC(), c.ID, userID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrInvalidChallenge
	}
	result, err = tx.ExecContext(ctx, `UPDATE withdrawal_security_profiles SET verified_email=?,email_verified_at=?,security_version=security_version+1 WHERE user_id=? AND security_version=?`, currentEmail, now.UTC(), userID, version)
	if err != nil {
		return err
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrInvalidChallenge
	}
	// Both changes commit or neither does; other old-version challenges expire.
	if err := recordAudit(ctx, tx, userID, version+1, "email_verified", c.ID, now); err != nil {
		return err
	}
	return tx.Commit()
}

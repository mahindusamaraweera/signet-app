package totp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrInvalidOrReusedCode = errors.New("invalid or reused authenticator code")

var ErrInvalidCode = fmt.Errorf("%w: code does not match", ErrInvalidOrReusedCode)
var ErrReusedCode = fmt.Errorf("%w: time step already consumed", ErrInvalidOrReusedCode)

type counterExecutor interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

// ConsumeWithdrawalCode must run in the same SQL transaction as creation of
// the withdrawal authorization. The caller must lock the user row and verify
// confirmed enrollment/current secret before calling. Rollback releases the
// consumption; commit prevents concurrent or subsequent reuse. Never persist
// the submitted code or secret in an authorization record.
func ConsumeWithdrawalCode(ctx context.Context, tx *sql.Tx, userID uint64, secret, submitted string, at time.Time) error {
	if tx == nil {
		return errors.New("authenticator consumption requires a transaction")
	}
	return consumeWithdrawalCode(ctx, tx, userID, secret, submitted, at)
}

func consumeWithdrawalCode(ctx context.Context, tx counterExecutor, userID uint64, secret, submitted string, at time.Time) error {
	if userID == 0 {
		return ErrInvalidCode
	}
	counter, ok := MatchingStep(secret, submitted, at, -1)
	if !ok {
		return ErrInvalidCode
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_totp_counters (user_id, last_counter) VALUES (?, -1) ON DUPLICATE KEY UPDATE user_id = user_id`, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_totp_counters SET last_counter = ?, updated_at = CURRENT_TIMESTAMP WHERE user_id = ? AND last_counter < ?`, counter, userID, counter)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return ErrReusedCode
	}
	return nil
}

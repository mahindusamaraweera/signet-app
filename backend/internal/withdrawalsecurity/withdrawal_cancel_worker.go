package withdrawalsecurity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func (s *Service) lockCancellationIntent(ctx context.Context, tx *sql.Tx, id string, expected WithdrawalTerms) (WithdrawalAuthorization, string, sql.NullInt64, error) {
	var owner uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=? FOR UPDATE`, expected.UserID).Scan(&owner); err != nil {
		return WithdrawalAuthorization{}, "", sql.NullInt64{}, err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, expected.UserID, id)
	if err != nil || owner != expected.UserID || a.Status != "consumed" || a.Terms != expected {
		return WithdrawalAuthorization{}, "", sql.NullInt64{}, ErrWithdrawalAuthorization
	}
	var encoded, digest, status string
	var remote sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT terms,terms_digest,status,remote_withdrawal_id FROM withdrawal_submission_outbox WHERE authorization_id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&encoded, &digest, &status, &remote); err != nil {
		return WithdrawalAuthorization{}, "", remote, err
	}
	var terms WithdrawalTerms
	if json.Unmarshal([]byte(encoded), &terms) != nil || terms != expected || digest != expected.Digest() || (status != "cancel_requested" && status != "cancelled") || (remote.Valid && remote.Int64 <= 0) {
		return WithdrawalAuthorization{}, "", remote, ErrWithdrawalAuthorization
	}
	return a, status, remote, nil
}

func (s *Service) BuildWithdrawalCancellation(ctx context.Context, id string, expected WithdrawalTerms, key []byte, now time.Time) (WithdrawalAssertion, string, error) {
	if !challengeIDPattern.MatchString(id) || len(key) < 32 || bytes.Equal(key, s.key) {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	defer tx.Rollback()
	a, status, _, err := s.lockCancellationIntent(ctx, tx, id, expected)
	if err != nil || status != "cancel_requested" {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	if err := s.bindAuthorizationUUID(ctx, tx, &a); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	assertion, signature, err := signWithdrawalCancellation(a, key, now)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	return assertion, signature, nil
}

// Remote nil is an explicitly confirmed tombstone without reservation. A known
// remote ID can never disappear; an unknown ID can be recovered from token.
func (s *Service) RecordWithdrawalCancellation(ctx context.Context, id string, expected WithdrawalTerms, remoteID *uint64, now time.Time) error {
	if !challengeIDPattern.MatchString(id) || (remoteID != nil && (*remoteID == 0 || *remoteID > uint64(1<<63-1))) {
		return ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, status, stored, err := s.lockCancellationIntent(ctx, tx, id, expected)
	if err != nil {
		return err
	}
	if stored.Valid && (remoteID == nil || uint64(stored.Int64) != *remoteID) {
		return ErrWithdrawalAuthorization
	}
	if status == "cancelled" {
		if !stored.Valid && remoteID != nil {
			return ErrWithdrawalAuthorization
		}
		return tx.Commit()
	}
	var approved sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT remote_withdrawal_id FROM withdrawal_authorizations WHERE id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&approved); err != nil {
		return err
	}
	if approved.Valid && (approved.Int64 <= 0 || remoteID == nil || uint64(approved.Int64) != *remoteID) {
		return ErrWithdrawalAuthorization
	}
	var value interface{}
	if remoteID != nil {
		value = *remoteID
	}
	result, err := tx.ExecContext(ctx, `UPDATE withdrawal_submission_outbox SET status='cancelled',remote_withdrawal_id=?,last_error=NULL,updated_at=? WHERE authorization_id=? AND user_id=? AND status='cancel_requested'`, value, now.UTC(), id, expected.UserID)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return ErrWithdrawalAuthorization
	}
	if remoteID != nil && !approved.Valid {
		result, err := tx.ExecContext(ctx, `UPDATE withdrawal_authorizations SET remote_withdrawal_id=? WHERE id=? AND user_id=? AND status='consumed' AND remote_withdrawal_id IS NULL`, value, id, expected.UserID)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return ErrWithdrawalAuthorization
		}
	}
	if err := recordAudit(ctx, tx, expected.UserID, expected.SecurityVersion, "withdrawal_cancelled", id, now); err != nil {
		return err
	}
	return tx.Commit()
}

package withdrawalsecurity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"time"
)

// BuildQueuedWithdrawalAssertion is server-worker only. It grants no authority
// from supplied terms alone: current security, durable consumed approval and
// matching pending outbox must all agree under the common SQL lock order.
// It does not send funds or persist credentials. Token must independently run
// its custody audit and revalidate source security before broadcast.
func (s *Service) BuildQueuedWithdrawalAssertion(ctx context.Context, id string, expected WithdrawalTerms, signingKey []byte, now time.Time) (WithdrawalAssertion, string, error) {
	if !challengeIDPattern.MatchString(id) || len(signingKey) < 32 || bytes.Equal(signingKey, s.key) {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	defer tx.Rollback()
	if _, _, err := s.lockWithdrawalCurrentState(ctx, tx, expected, nil, now); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	revision, err := s.withdrawalCredentialRevision(ctx, tx, expected.UserID)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, expected.UserID, id)
	if err != nil || a.Terms != expected || a.Status != "consumed" {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	var owner, version uint64
	var encoded, digest, status string
	var storedRevision sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT user_id,security_version,terms,terms_digest,status,credential_revision FROM withdrawal_submission_outbox WHERE authorization_id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&owner, &version, &encoded, &digest, &status, &storedRevision); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	var stored WithdrawalTerms
	if json.Unmarshal([]byte(encoded), &stored) != nil || stored != expected || owner != expected.UserID || version != expected.SecurityVersion || digest != expected.Digest() || status != "pending" || !storedRevision.Valid || !hmac.Equal([]byte(revision), []byte(storedRevision.String)) {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	if err := s.bindAuthorizationUUID(ctx, tx, &a); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	assertion, signature, err := signWithdrawalAssertion(a, signingKey, now)
	if err != nil {
		return WithdrawalAssertion{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return WithdrawalAssertion{}, "", err
	}
	return assertion, signature, nil
}

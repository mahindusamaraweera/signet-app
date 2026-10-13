package withdrawalsecurity

import (
	"bytes"
	"context"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"time"
)

// Server-to-server callback only. Freshly validates every security predicate,
// queued credential revision and BOTH durable remote identity links. This does
// not itself grant custody broadcast authority or replace financial auditing.
func (s *Service) BuildWithdrawalCurrentProof(ctx context.Context, id string, expected WithdrawalTerms, remoteID uint64, nonce string, signingKey []byte, now time.Time) (WithdrawalRevalidation, error) {
	if remoteID == 0 || !challengeIDPattern.MatchString(id) || !challengeIDPattern.MatchString(nonce) || len(signingKey) < 32 || bytes.Equal(signingKey, s.key) {
		return WithdrawalRevalidation{}, ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return WithdrawalRevalidation{}, err
	}
	defer tx.Rollback()
	if _, _, err := s.lockWithdrawalCurrentState(ctx, tx, expected, nil, now); err != nil {
		return WithdrawalRevalidation{}, err
	}
	revision, err := s.withdrawalCredentialRevision(ctx, tx, expected.UserID)
	if err != nil {
		return WithdrawalRevalidation{}, err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, expected.UserID, id)
	if err != nil || a.Terms != expected || a.Status != "consumed" {
		return WithdrawalRevalidation{}, ErrWithdrawalAuthorization
	}
	var owner, version uint64
	var encoded, digest, status string
	var storedRevision sql.NullString
	var queuedRemote sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT user_id,security_version,terms,terms_digest,status,credential_revision,remote_withdrawal_id FROM withdrawal_submission_outbox WHERE authorization_id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&owner, &version, &encoded, &digest, &status, &storedRevision, &queuedRemote); err != nil {
		return WithdrawalRevalidation{}, err
	}
	var stored WithdrawalTerms
	if json.Unmarshal([]byte(encoded), &stored) != nil || stored != expected || owner != expected.UserID || version != expected.SecurityVersion || digest != expected.Digest() || status != "submitted" || !storedRevision.Valid || !hmac.Equal([]byte(revision), []byte(storedRevision.String)) || !queuedRemote.Valid || queuedRemote.Int64 <= 0 || uint64(queuedRemote.Int64) != remoteID {
		return WithdrawalRevalidation{}, ErrWithdrawalAuthorization
	}
	var approvedRemote sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT remote_withdrawal_id FROM withdrawal_authorizations WHERE id=? AND user_id=? FOR UPDATE`, id, expected.UserID).Scan(&approvedRemote); err != nil {
		return WithdrawalRevalidation{}, err
	}
	if !approvedRemote.Valid || approvedRemote.Int64 <= 0 || uint64(approvedRemote.Int64) != remoteID {
		return WithdrawalRevalidation{}, ErrWithdrawalAuthorization
	}
	proof, err := signWithdrawalRevalidation(a, remoteID, nonce, signingKey, now)
	if err != nil {
		return WithdrawalRevalidation{}, err
	}
	if err := tx.Commit(); err != nil {
		return WithdrawalRevalidation{}, err
	}
	return proof, nil
}

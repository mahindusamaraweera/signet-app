package withdrawalsecurity

import (
	"context"
	"encoding/json"
	"time"
)

// The local queue is NOT a custody reservation or completed withdrawal. A
// worker must revalidate security and create a token request under stable
// authorization identity, retaining token financial audit before any payout.
func (s *Service) QueueWithdrawal(ctx context.Context, userID uint64, password, id, proof string, expected WithdrawalTerms, now time.Time) error {
	if userID == 0 || expected.UserID != userID || !challengeIDPattern.MatchString(id) {
		return ErrWithdrawalAuthorization
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, _, err := s.lockWithdrawalState(ctx, tx, expected, password, now); err != nil {
		return err
	}
	revision, err := s.withdrawalCredentialRevision(ctx, tx, userID)
	if err != nil {
		return err
	}
	a, err := loadWithdrawalAuthorization(ctx, tx, userID, id)
	if err != nil {
		return err
	}
	if err := consumeWithdrawalAuthorization(ctx, tx, a, s.key, proof, expected, now); err != nil {
		return err
	}
	encoded, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO withdrawal_submission_outbox (authorization_id,user_id,security_version,terms,terms_digest,status,created_at,updated_at,credential_revision) VALUES (?,?,?,?,?,'pending',?,?,?)`, id, userID, expected.SecurityVersion, string(encoded), expected.Digest(), now.UTC(), now.UTC(), revision); err != nil {
		return err
	}
	if err := recordAudit(ctx, tx, userID, expected.SecurityVersion, "withdrawal_queued", id, now); err != nil {
		return err
	}
	return tx.Commit()
}

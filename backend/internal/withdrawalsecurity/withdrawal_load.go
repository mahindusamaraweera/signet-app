package withdrawalsecurity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

var ErrWithdrawalAuthorization = errors.New("invalid withdrawal authorization")

// Caller first locks owner/profile/address in the shared security lock order.
// No read of this record alone grants financial authority.
func loadWithdrawalAuthorization(ctx context.Context, tx *sql.Tx, userID uint64, id string) (WithdrawalAuthorization, error) {
	if tx == nil || userID == 0 || !challengeIDPattern.MatchString(id) {
		return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
	}
	var a WithdrawalAuthorization
	var owner, version uint64
	var addressID, encoded, digest string
	var authorized, consumed sql.NullTime
	err := tx.QueryRowContext(ctx, `SELECT id,user_id,security_version,address_id,terms,terms_digest,proof_hash,status,expires_at,authorized_at,consumed_at FROM withdrawal_authorizations WHERE id=? AND user_id=? FOR UPDATE`, id, userID).Scan(&a.ID, &owner, &version, &addressID, &encoded, &digest, &a.ProofHash, &a.Status, &a.ExpiresAt, &authorized, &consumed)
	if err != nil {
		return WithdrawalAuthorization{}, err
	}
	if json.Unmarshal([]byte(encoded), &a.Terms) != nil {
		return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
	}
	canonical, err := NewWithdrawalTerms(owner, version, addressID, a.Terms.Destination, a.Terms.Gross)
	if err != nil || canonical != a.Terms || a.ID != id || owner != userID || a.Terms.Digest() != digest || !challengeIDPattern.MatchString(a.ProofHash) || a.ExpiresAt.IsZero() {
		return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
	}
	if authorized.Valid {
		a.AuthorizedAt = &authorized.Time
	}
	if consumed.Valid {
		a.ConsumedAt = &consumed.Time
	}
	switch a.Status {
	case "pending":
		if a.AuthorizedAt != nil || a.ConsumedAt != nil {
			return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
		}
	case "authorized":
		if a.AuthorizedAt == nil || a.ConsumedAt != nil || !a.AuthorizedAt.Before(a.ExpiresAt) {
			return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
		}
	case "consumed":
		if a.AuthorizedAt == nil || a.ConsumedAt == nil || a.ConsumedAt.Before(*a.AuthorizedAt) || !a.AuthorizedAt.Before(a.ExpiresAt) {
			return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
		}
	default:
		return WithdrawalAuthorization{}, ErrWithdrawalAuthorization
	}
	return a, nil
}

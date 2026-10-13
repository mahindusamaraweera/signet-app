package withdrawalsecurity

import (
	"context"
	"database/sql"
	"time"
)

type Address struct {
	ID          string     `json:"id"`
	Network     string     `json:"network"`
	Asset       string     `json:"asset"`
	Destination string     `json:"address"`
	VerifiedAt  time.Time  `json:"verified_at"`
	EligibleAt  time.Time  `json:"eligible_at"`
	RevokedAt   *time.Time `json:"revoked_at"`
}

// ListAddresses projects only public enrollment evidence for the session owner.
// An elapsed address wait alone is not withdrawal authorization.
func (s *Service) ListAddresses(ctx context.Context, userID uint64) ([]Address, error) {
	if userID == 0 {
		return nil, ErrSecurityCredentials
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,network,asset,address,verified_at,eligible_at,revoked_at FROM withdrawal_security_addresses WHERE user_id=? ORDER BY created_at DESC,id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Address, 0)
	for rows.Next() {
		var address Address
		var revoked sql.NullTime
		if err := rows.Scan(&address.ID, &address.Network, &address.Asset, &address.Destination, &address.VerifiedAt, &address.EligibleAt, &revoked); err != nil {
			return nil, err
		}
		if revoked.Valid {
			address.RevokedAt = &revoked.Time
		}
		result = append(result, address)
	}
	return result, rows.Err()
}

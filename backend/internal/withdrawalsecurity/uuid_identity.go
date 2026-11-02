package withdrawalsecurity

import (
	"context"
	"database/sql"
)

func (s *Service) RequireUUIDIdentity() { s.requireUUID = true }
func (s *Service) bindAuthorizationUUID(ctx context.Context, tx *sql.Tx, a *WithdrawalAuthorization) error {
	if !s.requireUUID {
		return nil
	}
	if err := tx.QueryRowContext(ctx, "SELECT uuid FROM users WHERE id=?", a.Terms.UserID).Scan(&a.ExternalUUID); err != nil {
		return err
	}
	if a.ExternalUUID == "" {
		return ErrWithdrawalAuthorization
	}
	return nil
}

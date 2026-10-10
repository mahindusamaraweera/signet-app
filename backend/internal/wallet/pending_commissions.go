package wallet

import (
	"errors"
	"github.com/jmoiron/sqlx"
	"sync/atomic"
)

var pendingCommissionGuard atomic.Bool
var ErrPendingCommissions = errors.New("pending blockchain commissions must settle before this legacy wallet credit; retry after settlement")

// Configure once at startup; legacy-only deployments never query integration tables.
func ConfigurePendingCommissionGuard(enabled bool) { pendingCommissionGuard.Store(enabled) }

func EnsureNoPendingCommissions(tx *sqlx.Tx, userID uint64) error {
	if !pendingCommissionGuard.Load() || userID == 1 {
		return nil
	}
	var id uint64
	if err := tx.Get(&id, "SELECT id FROM users WHERE id=? FOR UPDATE", userID); err != nil {
		return err
	}
	var pending int
	if err := tx.Get(&pending, "SELECT COUNT(*) FROM blockchain_topup_commission_plans WHERE beneficiary_user_id=? AND status='reserved'", userID); err != nil {
		return err
	}
	if pending != 0 {
		return ErrPendingCommissions
	}
	return nil
}

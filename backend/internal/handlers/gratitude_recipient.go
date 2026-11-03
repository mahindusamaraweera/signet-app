package handlers

import (
	"fmt"
	"github.com/jmoiron/sqlx"
)

// A signup may create an intermediate gratitude row for an existing upline,
// while the buyer's own row remains active. The package-bound log preserves
// that entitlement. Both representations describe ONE commission, not two.
func firstGratitudeRecipient(tx *sqlx.Tx, buyer, purchase uint64, node string, virtual uint64) (uint64, error) {
	var recipients []uint64
	if err := tx.Select(&recipients, `SELECT current_user_id FROM super_parent_logs WHERE gratitude_user=? AND user_package=? ORDER BY id FOR UPDATE`, buyer, purchase); err != nil {
		return 0, err
	}
	if len(recipients) > 1 {
		return 0, fmt.Errorf("ambiguous package gratitude evidence")
	}
	var recipient uint64
	if node == "gratitude" {
		if virtual == 0 {
			return 0, fmt.Errorf("invalid gratitude placement recipient")
		}
		recipient = virtual
	}
	if len(recipients) == 1 {
		if recipients[0] == 0 || (recipient != 0 && recipient != recipients[0]) {
			return 0, fmt.Errorf("conflicting package gratitude recipient")
		}
		recipient = recipients[0]
	}
	return recipient, nil
}

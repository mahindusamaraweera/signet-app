// mining_update.go ports `mining:update` — UpdateMiningTokens
// (financial_engine.md §5, accrues mining tokens; original run every
// minute). Per ARCHITECTURE.md, the original's Pusher MiningUpdated
// broadcast is intentionally NOT replicated here — the frontend polls the
// existing GET /api/v1/mining/user/{userId} endpoint instead, so this job
// only needs to persist state, not push it.
package jobs

import (
	"log"

	"github.com/jmoiron/sqlx"

	"signet-backend/internal/models"
)

// RunMiningUpdate accrues mining_token for every active, actively-mining
// user_minings row, matching (per row):
//
//	perMinute := daily_mining / 1440
//	newToken := mining_token + perMinute
//	if newToken >= total_token: newToken = total_token; status = 'inactive'
func RunMiningUpdate(db *sqlx.DB) error {
	var ids []uint64
	if err := db.Select(&ids, "SELECT id FROM user_minings WHERE status = 'active' AND daily_mining > 0 AND NOT EXISTS (SELECT 1 FROM historical_mining_cutovers c WHERE c.user_id=user_minings.user_id)"); err != nil {
		return err
	}
	for _, id := range ids {
		if err := miningUpdateRow(db, id); err != nil {
			log.Printf("jobs: mining:update: failed for user_minings id %d: %v", id, err)
		}
	}
	return nil
}

// miningUpdateRow re-fetches the row fresh (rather than reusing anything
// from the id-listing query above) — matches the original's "fresh fetch of
// that user's user_minings row" note.
func miningUpdateRow(db *sqlx.DB, id uint64) error {
	var m models.UserMining
	if err := db.Get(&m, "SELECT * FROM user_minings WHERE id = ?", id); err != nil {
		return err
	}
	if m.Status != "active" || m.DailyMining <= 0 {
		return nil
	}

	perMinute := float64(m.DailyMining) / 1440.0
	newToken := m.MiningToken + perMinute
	newStatus := m.Status
	if newToken >= float64(m.TotalToken) {
		newToken = float64(m.TotalToken)
		newStatus = "inactive"
	}

	_, err := db.Exec("UPDATE user_minings SET mining_token = ?, status = ?, updated_at = NOW() WHERE id = ? AND status='active' AND daily_mining=? AND total_token=? AND NOT EXISTS (SELECT 1 FROM historical_mining_cutovers c WHERE c.user_id=user_minings.user_id)", newToken, newStatus, id, m.DailyMining, m.TotalToken)
	return err
}

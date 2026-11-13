// share_calculate.go ports `share:calculate` — CalculateGlobalDirectorShare
// (financial_engine.md §5, monthly Global Director profit-share payout).
package jobs

import (
	"database/sql"
	"log"
	"math"
	"time"

	"github.com/jmoiron/sqlx"

	"signet-backend/internal/wallet"
)

// RunShareCalculate distributes the current month's Global Director profit
// share, gated by wallet.UpdateWallet's 4x-cap check (financial_engine.md
// §1) per recipient. Idempotent per calendar-month period: a period already
// recorded in global_director_share_distributions is skipped outright; if
// any individual credit fails mid-run the period is NOT marked distributed,
// so a safe re-run will re-pay everyone (including already-paid users — no
// per-user idempotency, only whole-period, matching the original verbatim).
//
// The original pays now()'s month, i.e. it was run at the END of the month.
// This runs daily, so it pays the most recent COMPLETED month instead —
// same pool, same period label — rather than paying the current month on
// its first day with a partial pool and marking the month done.
func RunShareCalculate(db *sqlx.DB) error {
	now := time.Now()
	lastMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
	period := lastMonth.Format("2006-01")

	var alreadyDistributed int
	if err := db.Get(&alreadyDistributed, "SELECT COUNT(*) FROM global_director_share_distributions WHERE period = ?", period); err != nil {
		return err
	}
	if alreadyDistributed > 0 {
		return nil
	}

	var totalPool sql.NullFloat64
	if err := db.Get(&totalPool, `
		SELECT COALESCE(SUM(pool_amount), 0) FROM package_pools
		WHERE YEAR(created_at) = ? AND MONTH(created_at) = ? AND pool_amount > 0`,
		lastMonth.Year(), int(lastMonth.Month())); err != nil {
		return err
	}
	if totalPool.Float64 <= 0 {
		return nil
	}

	var totalShares sql.NullFloat64
	if err := db.Get(&totalShares, `SELECT COALESCE(SUM(global_director_share), 0) FROM users WHERE global_director_share_status = 1`); err != nil {
		return err
	}
	if totalShares.Float64 <= 0 {
		return nil
	}

	valuePerShare := totalPool.Float64 / totalShares.Float64

	type shareUser struct {
		ID                  uint64  `db:"id"`
		GlobalDirectorShare float64 `db:"global_director_share"`
	}
	var users []shareUser
	if err := db.Select(&users, `SELECT id, global_director_share FROM users WHERE global_director_share_status = 1`); err != nil {
		return err
	}

	logAt := sharePeriodLogTime(lastMonth)

	succeeded := 0
	anyFailed := false
	for _, u := range users {
		amount := math.Round(u.GlobalDirectorShare*valuePerShare*100) / 100
		if err := wallet.UpdateWalletAt(db, u.ID, amount, "Global Director Share", logAt); err != nil {
			log.Printf("jobs: share:calculate: credit failed for user %d: %v", u.ID, err)
			anyFailed = true
			continue
		}
		succeeded++
	}

	if anyFailed {
		return nil // period stays undistributed, safe to re-run later
	}

	_, err := db.Exec(`INSERT INTO global_director_share_distributions (period, total_pool, total_shares, value_per_share, user_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, NOW(), NOW())`,
		period, totalPool.Float64, totalShares.Float64, valuePerShare, succeeded)
	return err
}

// sharePeriodLogTime is the created_at stamped on a period's GDS log rows:
// the period's last day at 23:59:59 Asia/Colombo, which is when the original
// month-end cron paid out (e.g. 2026-09-30 18:29 UTC). The job itself runs
// after the month closes, so without this the payout would be logged in the
// following month and drop out of that period's date-filtered reports.
func sharePeriodLogTime(periodStart time.Time) time.Time {
	colombo := time.FixedZone("Asia/Colombo", 19800)
	y, m, _ := periodStart.Date()
	return time.Date(y, m+1, 1, 0, 0, 0, 0, colombo).Add(-time.Second).UTC()
}

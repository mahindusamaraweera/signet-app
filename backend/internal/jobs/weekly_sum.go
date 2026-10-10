// weekly_sum.go ports `packages:weekly-sum` — WeeklyActivePackageSum
// (financial_engine.md §5, weekly ROC distribution, Monday-Sunday).
//
// Scheduling note: StartScheduler (scheduler.go) invokes RunWeeklySum once a
// day rather than pinning it to a Monday-only cron entry. RunWeeklySum
// itself computes the bounds of the most recently COMPLETED Monday-Sunday
// week (i.e. as of "now", the week before the current one) and is
// idempotent per week via a weekly_package_summaries.week_start existence
// check — so a daily tick naturally no-ops on every day except the first
// one after a week closes, and a missed run (e.g. process restart) is
// self-healing on the next day's tick rather than silently skipped forever.
package jobs

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
)

func RunWeeklySum(db *sqlx.DB) error {
	// One transaction for the whole week: the already-paid check, every
	// credit and log row, and the summary row that marks the week paid.
	// Any failure rolls all of it back, so a week can never be half paid
	// and then paid again by the next daily check.
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	isoWeekday := int(now.Weekday())
	if isoWeekday == 0 {
		isoWeekday = 7 // Go's Weekday() is Sunday=0; ISO wants Monday=1..Sunday=7
	}
	thisMonday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(isoWeekday - 1))
	weekStart := thisMonday.AddDate(0, 0, -7)
	weekEndDay := thisMonday.AddDate(0, 0, -1)
	weekEnd := time.Date(weekEndDay.Year(), weekEndDay.Month(), weekEndDay.Day(), 23, 59, 59, 0, now.Location())

	weekStartStr := weekStart.Format("2006-01-02")
	weekEndStr := weekEnd.Format("2006-01-02")

	var alreadyProcessed int
	if err := tx.Get(&alreadyProcessed, "SELECT COUNT(*) FROM weekly_package_summaries WHERE DATE(week_start) = ? FOR UPDATE", weekStartStr); err != nil {
		return err
	}
	if alreadyProcessed > 0 {
		return nil
	}

	var weekTotal sql.NullFloat64
	if err := tx.Get(&weekTotal, `
		SELECT COALESCE(SUM(p.price), 0)
		FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up.status = 'active' AND up.activated_at BETWEEN ? AND ?`, weekStart, weekEnd); err != nil {
		return err
	}

	perWeekTotal := weekTotal.Float64 * 0.05
	// balance_forward carry-over is CODE-DISABLED in the original (commented
	// out) — do not carry forward, even though the column still gets
	// written (see financial_engine.md §5).
	total := perWeekTotal
	if total <= 0 {
		return nil
	}

	isoYear, isoWeek := weekStart.ISOWeek()
	jobID := fmt.Sprintf("JOB-%dW%d-%s", isoYear, isoWeek, jobsRandomUpper(5))

	type rocUser struct {
		ID         uint64  `db:"id"`
		RocBalance float64 `db:"roc_balance"`
	}
	var activeRoc []rocUser
	if err := tx.Select(&activeRoc, `
		SELECT u.id, COALESCE(w.roc_balance, 0) AS roc_balance
		FROM users u LEFT JOIN wallets w ON w.user_id = u.id
		WHERE u.roc_status = 'active'`); err != nil {
		return err
	}

	var rocPotation float64
	if len(activeRoc) > 0 {
		rocPotation = total / float64(len(activeRoc))
	}

	var remainingBalance float64
	paid := 0
	for _, u := range activeRoc {
		var pkg struct {
			Price int64 `db:"price"`
		}
		err := tx.Get(&pkg, `
			SELECT p.price FROM user_packages up JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id = ? AND up.status = 'active' ORDER BY up.id ASC LIMIT 1`, u.ID) // ->first(): lowest id, as the original
		if err == sql.ErrNoRows {
			continue // skip user if no active package
		}
		if err != nil {
			return fmt.Errorf("week %s: load active package for user %d: %w (nothing was paid)", weekStartStr, u.ID, err)
		}

		var maxPayout float64
		switch pkg.Price {
		case 100:
			maxPayout = 5
		case 500:
			maxPayout = 25
		case 1000:
			maxPayout = 50
		default:
			maxPayout = 0
		}

		if u.RocBalance >= float64(pkg.Price)*5 {
			if _, err := tx.Exec("UPDATE users SET roc_status = 'stopped', updated_at = NOW() WHERE id = ?", u.ID); err != nil {
				return fmt.Errorf("week %s: stop roc_status for user %d: %w (nothing was paid)", weekStartStr, u.ID, err)
			}
			continue // lifetime 5x package-price ROC cap reached, no payout this run
		}

		var payout float64
		if rocPotation > maxPayout {
			payout = maxPayout
			remainingBalance += rocPotation - maxPayout // accumulated but NOT reused this run, only stored for audit
		} else {
			payout = rocPotation
		}

		// Credits wallets.roc_balance DIRECTLY via raw SQL, NOT through
		// wallet.UpdateWallet — ROC is a separate ledger from wallet.balance
		// and is not subject to the 4x earnings cap gate (financial_engine.md §5).
		// Any failure aborts the whole week (rolled back, retried on the next
		// daily check) rather than skipping one user and marking it paid.
		if err := weeklySumCreditRoc(tx, u.ID, payout); err != nil {
			return fmt.Errorf("week %s: credit roc_balance for user %d: %w (nothing was paid)", weekStartStr, u.ID, err)
		}
		if err := weeklySumStoreEarnLog(tx, u.ID, payout, jobID); err != nil {
			return fmt.Errorf("week %s: store earn log for user %d: %w (nothing was paid)", weekStartStr, u.ID, err)
		}
		paid++
	}

	if _, err := tx.Exec(`INSERT INTO weekly_package_summaries (job_id, week_start, week_end, per_week_total, balance_forward, roc_potation, total_amount, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`,
		jobID, weekStartStr, weekEndStr, weekTotal.Float64, remainingBalance, uint64(math.Floor(rocPotation)), total); err != nil {
		return fmt.Errorf("week %s: record summary: %w (nothing was paid)", weekStartStr, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("week %s: commit: %w (nothing was paid)", weekStartStr, err)
	}
	log.Printf("jobs: packages:weekly-sum: paid ROC for week %s..%s (%s) to %d users, %.4f each", weekStartStr, weekEndStr, jobID, paid, rocPotation)
	return nil
}

func weeklySumCreditRoc(db *sqlx.Tx, userID uint64, amount float64) error {
	var walletID sql.NullInt64
	err := db.Get(&walletID, "SELECT id FROM wallets WHERE user_id = ?", userID)
	switch {
	case err == sql.ErrNoRows:
		_, err = db.Exec("INSERT INTO wallets (user_id, balance, roc_balance, created_at, updated_at) VALUES (?, 0, ?, NOW(), NOW())", userID, amount)
		return err
	case err != nil:
		return err
	default:
		_, err = db.Exec("UPDATE wallets SET roc_balance = roc_balance + ?, updated_at = NOW() WHERE user_id = ?", amount, userID)
		return err
	}
}

func weeklySumStoreEarnLog(db *sqlx.Tx, userID uint64, amount float64, jobID string) error {
	res, err := db.Exec(`INSERT INTO earn_logs (user_id, amount, description, created_at, updated_at) VALUES (?, ?, 'ROC Income', NOW(), NOW())`,
		jobsItoa(userID), jobsFtoa(amount))
	if err != nil {
		return err
	}
	earnLogID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO roc_income_log (job_id, user_id, amount, description, status, earn_log_id, created_at, updated_at)
		VALUES (?, ?, ?, 'ROC Income', 'pending', ?, NOW(), NOW())`, jobID, userID, amount, earnLogID)
	return err
}

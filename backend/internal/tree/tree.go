// Package tree ports the MLM genealogy placement algorithm and the rank/roc
// dashboard-widget helpers from the original app/helpers.php, documented in
// docs/analysis/financial_engine.md §2 and §4. This is the most
// intricate/quirky part of the original codebase — every branch here is a
// deliberate line-for-line port, including behavior that looks odd (e.g.
// superParentFind keying off user_id where ParentFind keys off virtual_id).
// Do not "clean up" the control flow without re-checking against
// financial_engine.md first.
package tree

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"signet-backend/internal/wallet"
)

// nodes eligible as "active" per the various whereIn(...) clauses in the
// original.
var activeLikeNodes = []string{"active", "gratitude", "correct"}

// PlacementStore allows the unchanged placement algorithm to participate in
// the caller's transaction, including its intermediate tree/log writes.
type PlacementStore interface {
	Get(interface{}, string, ...interface{}) error
	Exec(string, ...interface{}) (sql.Result, error)
	Rebind(string) string
}

func placementCapCheck(db PlacementStore, userID, packageID uint64) (int, error) {
	switch store := db.(type) {
	case *sqlx.Tx:
		return wallet.CheckWalletTx(store, userID, packageID)
	case *sqlx.DB:
		return wallet.CheckWallet(store, userID, packageID)
	default:
		return 0, fmt.Errorf("unsupported placement cap store")
	}
}

// ParentFind ports ParentFind($id, $package, $currentUser, $i, &$createdIds).
// id is the current node being evaluated (starts as the referrer's id);
// currentUser is the user actually activating (used for super_parent_logs);
// i is the recursion-depth counter; createdIds accumulates ids of every
// user_parents row created along the way.
func ParentFind(db PlacementStore, id, packageID, currentUser uint64, i int, createdIds *[]uint64) (uint64, error) {
	if id == 1 {
		return 1, nil // root/company sentinel — matches User::where('id',1)->first()?.id
	}

	activeOrderCount, err := countUserParentByVirtual(db, id, activeLikeNodes)
	if err != nil {
		return 0, err
	}
	predefined := map[int]bool{1: true, 4: true, 9: true, 14: true, 19: true, 29: true, 39: true, 49: true, 74: true, 99: true}

	var parent *userParentRow
	parent, err = firstUserParentByVirtual(db, id, []string{"active", "deactive"})
	if err != nil {
		return 0, err
	}

	if parent != nil {
		active, statusErr := userIsInactive(db, parent.VirtualID)
		if statusErr != nil {
			return 0, statusErr
		}
		if active {
			return SuperParentFind(db, parent.VirtualID, packageID, currentUser, i, createdIds)
		}
	}

	switch {
	case predefined[activeOrderCount]:
		if parent == nil {
			return 0, nil
		}
		if parent.VirtualID != 2 && parent.VirtualID != 3 && parent.VirtualID != 4 {
			newID, err := insertUserParent(db, id, parent.VirtualID, parent.VirtualID, "gratitude")
			if err != nil {
				return 0, err
			}
			*createdIds = append(*createdIds, newID)
			return SuperParentFind(db, parent.VirtualID, packageID, currentUser, i, createdIds)
		}
		activation, err := placementCapCheck(db, parent.VirtualID, packageID)
		if err != nil {
			return 0, err
		}
		if activation == 1 {
			return parent.VirtualID, nil
		}
		return 1, nil

	case activeOrderCount > 100:
		nextMultipleOf10 := ceilToMultipleOf10Minus1(activeOrderCount)
		if activeOrderCount == nextMultipleOf10 {
			if parent == nil {
				return 0, nil
			}
			return SuperParentFind(db, parent.VirtualID, packageID, currentUser, i, createdIds)
		}
		return id, nil

	default:
		if activeOrderCount == 0 {
			return id, nil
		}
		if parent == nil {
			return 0, nil
		}
		activation, err := placementCapCheck(db, parent.VirtualID, packageID)
		if err != nil {
			return 0, err
		}
		if activation == 1 {
			return id, nil
		}
		return 1, nil
	}
}

// SuperParentFind ports superParentFind($virtualParent, $package, $currentUser, $i, &$createdIds).
func SuperParentFind(db PlacementStore, virtualParent, packageID, currentUser uint64, i int, createdIds *[]uint64) (uint64, error) {
	activeOrderCount, err := countUserParentByVirtual(db, virtualParent, activeLikeNodes)
	if err != nil {
		return 0, err
	}

	if _, err := checkAndLogFirstTimeSuper(db, virtualParent, packageID, currentUser, i); err != nil {
		return 0, err
	}

	predefined := map[int]bool{2: true, 5: true, 10: true, 15: true, 20: true, 30: true, 40: true, 50: true, 75: true, 100: true}

	parent, err := firstUserParentByUserID(db, virtualParent)
	if err != nil {
		return 0, err
	}
	i++

	if parent != nil {
		active, statusErr := userIsInactive(db, parent.UserID)
		if statusErr != nil {
			return 0, statusErr
		}
		if active {
			return ParentFind(db, parent.ParentID, packageID, currentUser, i, createdIds)
		}
	}

	switch {
	case predefined[activeOrderCount]:
		if parent == nil {
			return 0, nil
		}
		return ParentFind(db, parent.ParentID, packageID, currentUser, i, createdIds)

	case activeOrderCount > 100:
		if parent == nil {
			return 0, nil
		}
		nextMultipleOf10 := ceilToMultipleOf10Minus1(activeOrderCount)
		if activeOrderCount == nextMultipleOf10 {
			return ParentFind(db, parent.ParentID, packageID, currentUser, i, createdIds)
		}
		return parent.ParentID, nil

	default:
		if parent == nil {
			return 0, nil
		}
		activation, err := placementCapCheck(db, parent.VirtualID, packageID)
		if err != nil {
			return 0, err
		}
		if activation == 1 {
			return virtualParent, nil
		}
		return 1, nil
	}
}

func checkAndLogFirstTimeSuper(db PlacementStore, virtualParent, packageID, currentUser uint64, i int) (bool, error) {
	var exists int
	err := db.Get(&exists, `
		SELECT COUNT(*) FROM super_parent_logs
		WHERE current_user_id = ? AND package_id = ? AND gratitude_user = ? AND created_at >= ?`,
		virtualParent, packageID, currentUser, time.Now().Add(-5*time.Minute))
	if err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil
	}
	if i == 1 {
		_, err = db.Exec(`INSERT INTO super_parent_logs (current_user_id, package_id, gratitude_user, created_at, updated_at)
			VALUES (?, ?, ?, NOW(), NOW())`, virtualParent, packageID, currentUser)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

// --- small DB helpers ---

type userParentRow struct {
	ID        uint64 `db:"id"`
	UserID    uint64 `db:"user_id"`
	VirtualID uint64 `db:"virtual_id"`
	ParentID  uint64 `db:"parent_id"`
	Node      string `db:"node"`
}

func countUserParentByVirtual(db PlacementStore, virtualID uint64, nodes []string) (int, error) {
	query, args, err := sqlx.In("SELECT COUNT(*) FROM user_parents WHERE virtual_id = ? AND node IN (?)", virtualID, nodes)
	if err != nil {
		return 0, err
	}
	query = db.Rebind(query)
	var count int
	if err := db.Get(&count, query, args...); err != nil {
		return 0, err
	}
	return count, nil
}

func firstUserParentByVirtual(db PlacementStore, virtualID uint64, nodes []string) (*userParentRow, error) {
	query, args, err := sqlx.In("SELECT id, user_id, virtual_id, parent_id, node FROM user_parents WHERE virtual_id = ? AND node IN (?) LIMIT 1", virtualID, nodes)
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)
	var row userParentRow
	if err := db.Get(&row, query, args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

func firstUserParentByUserID(db PlacementStore, userID uint64) (*userParentRow, error) {
	var row userParentRow
	err := db.Get(&row, "SELECT id, user_id, virtual_id, parent_id, node FROM user_parents WHERE user_id = ? LIMIT 1", userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func insertUserParent(db PlacementStore, userID, virtualID, parentID uint64, node string) (uint64, error) {
	res, err := db.Exec(`INSERT INTO user_parents (user_id, virtual_id, parent_id, node, created_at, updated_at)
		VALUES (?, ?, ?, ?, NOW(), NOW())`, userID, virtualID, parentID, node)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return uint64(id), err
}

// userIsInactive reports whether the users row with this id has
// status = 'inactive' (false/no-error if the user doesn't exist, matching
// the original's `$checkActive ? $checkActive->status : 'Not Found'`
// defensive read).
func userIsInactive(db PlacementStore, userID uint64) (bool, error) {
	var status sql.NullString
	err := db.Get(&status, "SELECT status FROM users WHERE id = ?", userID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return status.String == "inactive", nil
}

func ceilToMultipleOf10Minus1(n int) int {
	return ((n + 9) / 10 * 10) - 1
}

// --- Rank ---

type RankResult struct {
	TeamSalesDirect float64 `json:"team_sales_direct"`
	Gratitude       float64 `json:"gratitude"`
	CurrentRank     string  `json:"current_rank"`
	NextRank        *string `json:"next_rank"`
	RemainingTeam   float64 `json:"remaining_team"`
	RemainingSuper  float64 `json:"remaining_super"`
}

type rankTier struct {
	Name  string
	Team  float64
	Super float64
}

// rankLadder mirrors the hardcoded $ranks map in helpers.php's rank(),
// preserved verbatim including order (first tier met, in this order, wins).
var rankLadder = []rankTier{
	{"Crystal", 5000, 100},
	{"Jade", 10000, 200},
	{"Emerald", 20000, 300},
	{"Ruby", 30000, 500},
	{"Diamond", 100000, 1000},
	{"Senior Diamond", 250000, 2000},
	{"Senior Executive Diamond", 500000, 5000},
	{"Crown Diamond", 1000000, 10000},
}

// gratitudeCutoff is the hardcoded date used by rank()'s activeUsersGrant
// query — preserved verbatim, see financial_engine.md §4.
var gratitudeCutoff = "2025-11-22"

// teamSalesCutoff is the hardcoded date used by rank()'s
// totalActivePackagesDirect join — preserved verbatim.
var teamSalesCutoff = "2025-10-01"

// Rank ports rank($user_id) from helpers.php, returning structured data
// instead of a raw HTML string (the Vue frontend renders the badge row
// itself — see ui_spec.md's "Helper-driven inline widgets" section).
func Rank(db *sqlx.DB, userID uint64) (*RankResult, error) {
	var activeDirect []uint64
	if err := db.Select(&activeDirect, `SELECT user_id FROM user_parents WHERE virtual_id = ? AND node = 'active'`, userID); err != nil {
		return nil, err
	}
	var activeGrant []uint64
	if err := db.Select(&activeGrant, `SELECT user_id FROM user_parents WHERE parent_id = ? AND node = 'gratitude' AND DATE(created_at) >= ?`, userID, gratitudeCutoff); err != nil {
		return nil, err
	}
	activeUsers := dedupe(append(append([]uint64{}, activeDirect...), activeGrant...))

	var totalActivePackages float64
	if len(activeUsers) > 0 {
		query, args, err := sqlx.In(`
			SELECT COALESCE(SUM(p.price), 0)
			FROM user_packages up
			JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id IN (?) AND up.status = 'active'`, activeUsers)
		if err != nil {
			return nil, err
		}
		query = db.Rebind(query)
		if err := db.Get(&totalActivePackages, query, args...); err != nil {
			return nil, err
		}
	}

	var totalActivePackagesDirect float64
	if err := db.Get(&totalActivePackagesDirect, `
		SELECT COALESCE(SUM(p.price), 0)
		FROM user_packages up
		JOIN user_parents up2 ON up2.user_id = up.user_id
		JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
		WHERE up2.virtual_id = ? AND up2.node IN ('active','correct')
		  AND DATE(up2.created_at) >= ? AND up.status = 'active'`, userID, teamSalesCutoff); err != nil {
		return nil, err
	}

	var gratuityUsers []uint64
	if err := db.Select(&gratuityUsers, `SELECT gratitude_user FROM super_parent_logs WHERE current_user_id = ?`, userID); err != nil {
		return nil, err
	}
	var totalActiveSuperPackages float64
	if len(gratuityUsers) > 0 {
		query, args, err := sqlx.In(`
			SELECT COALESCE(SUM(p.price * 0.2), 0)
			FROM user_packages up
			JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id IN (?) AND up.status = 'active'`, gratuityUsers)
		if err != nil {
			return nil, err
		}
		query = db.Rebind(query)
		if err := db.Get(&totalActiveSuperPackages, query, args...); err != nil {
			return nil, err
		}
	}

	result := &RankResult{
		TeamSalesDirect: totalActivePackagesDirect,
		Gratitude:       totalActiveSuperPackages,
		CurrentRank:     "No Rank",
	}
	for _, tier := range rankLadder {
		if totalActivePackages >= tier.Team && totalActiveSuperPackages >= tier.Super {
			result.CurrentRank = tier.Name
			result.NextRank = nil
			result.RemainingTeam = 0
			result.RemainingSuper = 0
			continue
		}
		next := tier.Name
		result.NextRank = &next
		result.RemainingTeam = maxf(tier.Team-totalActivePackages, 0)
		result.RemainingSuper = maxf(tier.Super-totalActiveSuperPackages, 0)
		break
	}
	return result, nil
}

func dedupe(ids []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// --- Roc widget ---

type RocSummary struct {
	WeekStart    *string `json:"week_start"`
	WeekEnd      *string `json:"week_end"`
	PerWeekTotal *string `json:"per_week_total"`
}

// Roc ports roc($user_id): the most recent RocIncomeLog for the user,
// joined to its WeeklyPackageSummary by job_id.
func Roc(db *sqlx.DB, userID uint64) (*RocSummary, error) {
	var jobID sql.NullString
	err := db.Get(&jobID, `SELECT job_id FROM roc_income_log WHERE user_id = ? ORDER BY created_at DESC LIMIT 1`, userID)
	if err == sql.ErrNoRows {
		return &RocSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	var summary struct {
		WeekStart    sql.NullTime    `db:"week_start"`
		WeekEnd      sql.NullTime    `db:"week_end"`
		PerWeekTotal sql.NullFloat64 `db:"per_week_total"`
	}
	err = db.Get(&summary, `SELECT week_start, week_end, per_week_total FROM weekly_package_summaries WHERE job_id = ? LIMIT 1`, jobID.String)
	if err == sql.ErrNoRows {
		return &RocSummary{}, nil
	}
	if err != nil {
		return nil, err
	}
	ws := summary.WeekStart.Time.Format("2006-01-02")
	we := summary.WeekEnd.Time.Format("2006-01-02")
	total := fmt.Sprintf("%.2f", summary.PerWeekTotal.Float64)
	return &RocSummary{WeekStart: &ws, WeekEnd: &we, PerWeekTotal: &total}, nil
}

// AllUsers ports allUsers(): count of active, role='user' accounts.
func AllUsers(db *sqlx.DB) (int, error) {
	var count int
	err := db.Get(&count, `SELECT COUNT(*) FROM users WHERE status = 'active' AND role = 'user'`)
	return count, err
}

// NewActivations ports newActivations(): count of user_packages awaiting
// company approval (company_status = 0).
func NewActivations(db *sqlx.DB) (int, error) {
	var count int
	err := db.Get(&count, `SELECT COUNT(*) FROM user_packages WHERE company_status = 0`)
	return count, err
}

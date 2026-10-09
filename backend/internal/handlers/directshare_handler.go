// directshare_handler.go covers DirectShareController (api_spec.md
// "## DirectShareController") — all role:company.
package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
)

func RegisterDirectShareRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/direct-share", directShareHandler(d))
		r.Get("/api/v1/direct-share-log", directShareLogHandler(d))
		r.Post("/api/v1/package-pools", packagePoolStoreHandler(d))
		r.Put("/api/v1/package-pools/{id}", packagePoolUpdateHandler(d))
		r.Delete("/api/v1/package-pools/{id}", packagePoolDestroyHandler(d))
	})
}

// --- shared date-range helpers (also used by leaderexecutive_handler.go) ---

// reportsDateFilter builds an " AND DATE(<col>) >= ? AND DATE(<col>) <= ?"
// SQL fragment (only the clauses that apply) plus matching args, for
// optional inbound start/end date query params.
func reportsDateFilter(col, startDate, endDate string) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if startDate != "" {
		clauses = append(clauses, "DATE("+col+") >= ?")
		args = append(args, startDate)
	}
	if endDate != "" {
		clauses = append(clauses, "DATE("+col+") <= ?")
		args = append(args, endDate)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// reportsPreviousMonthBounds returns the first and last calendar day of the
// PREVIOUS month relative to now — mirrors Laravel's
// `subMonthNoOverflow()->startOfMonth()/endOfMonth()` used as the default
// date range for direct-share-log and leadership-bonus-log.
func reportsPreviousMonthBounds() (time.Time, time.Time) {
	now := time.Now()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	lastOfPrevMonth := firstOfThisMonth.AddDate(0, 0, -1)
	firstOfPrevMonth := time.Date(lastOfPrevMonth.Year(), lastOfPrevMonth.Month(), 1, 0, 0, 0, 0, now.Location())
	return firstOfPrevMonth, lastOfPrevMonth
}

// reportsCurrentMonthBounds returns the first and last calendar day of the
// CURRENT month — default range for leaders.gain / executives.gain.
func reportsCurrentMonthBounds() (time.Time, time.Time) {
	now := time.Now()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	next := first.AddDate(0, 1, 0)
	last := next.AddDate(0, 0, -1)
	return first, last
}

const reportsDateLayout = "2006-01-02"

// --- GET /direct-share ---

func directShareHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startDate := r.URL.Query().Get("start_date")
		endDate := r.URL.Query().Get("end_date")

		dateWhere, dateArgs := reportsDateFilter("pp.created_at", startDate, endDate)

		var companyPool models.NullFloat64
		_ = d.DB.Get(&companyPool, "SELECT COALESCE(SUM(pp.pool_amount),0) FROM package_pools pp WHERE pp.user_id = 1"+dateWhere, dateArgs...)

		var salesPool models.NullFloat64
		_ = d.DB.Get(&salesPool, "SELECT COALESCE(SUM(pp.pool_amount),0) FROM package_pools pp WHERE pp.user_id != 1"+dateWhere, dateArgs...)

		totalPool := companyPool.Float64 + salesPool.Float64

		page, perPage, offset := httpx.PageParams(r, 20)

		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM package_pools pp WHERE 1=1"+dateWhere, dateArgs...)

		listArgs := append(append([]interface{}{}, dateArgs...), perPage, offset)
		rows, err := d.DB.Queryx(`
			SELECT pp.id, pp.user_id, pp.package_id, pp.pool_amount, pp.created_at,
			       u.name AS user_name, p.name AS package_name, p.price AS package_price
			FROM package_pools pp
			LEFT JOIN users u ON u.id = pp.user_id
			LEFT JOIN packages p ON p.id = pp.package_id
			WHERE 1=1`+dateWhere+`
			ORDER BY pp.created_at DESC LIMIT ? OFFSET ?`, listArgs...)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var pools []map[string]interface{}
		for rows.Next() {
			var id, userID, packageID uint64
			var poolAmount float64
			var createdAt models.NullTime
			var userName, packageName models.NullString
			var packagePrice models.NullInt64
			if err := rows.Scan(&id, &userID, &packageID, &poolAmount, &createdAt, &userName, &packageName, &packagePrice); err != nil {
				continue
			}
			pools = append(pools, map[string]interface{}{
				"id": id, "user_id": userID, "package_id": packageID, "pool_amount": poolAmount,
				"created_at": createdAt.Time, "user_name": userName.String, "package_name": packageName.String,
				// direct_share.blade.php's "Package Value" column: $pool->package->price ?? 'N/A'
				"package_price": packagePrice,
			})
		}

		httpx.OK(w, map[string]interface{}{
			"status":       "success",
			"company_pool": companyPool.Float64,
			"sales_pool":   salesPool.Float64,
			"total_pool":   totalPool,
			"start_date":   startDate,
			"end_date":     endDate,
			"pools":        httpx.Paginate(pools, total, page, perPage),
		})
	}
}

// --- GET /direct-share-log ---

func directShareLogHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defaultStart, defaultEnd := reportsPreviousMonthBounds()
		startDate := r.URL.Query().Get("start_date")
		if startDate == "" {
			startDate = defaultStart.Format(reportsDateLayout)
		}
		endDate := r.URL.Query().Get("end_date")
		if endDate == "" {
			endDate = defaultEnd.Format(reportsDateLayout)
		}

		page, perPage, offset := httpx.PageParams(r, 20)

		var total int
		_ = d.DB.Get(&total, `SELECT COUNT(*) FROM global_share_wallets_log WHERE DATE(created_at) >= ? AND DATE(created_at) <= ?`, startDate, endDate)

		rows, err := d.DB.Queryx(`
			SELECT gswl.id, gswl.user_id, gswl.amount, gswl.description, gswl.created_at, u.name AS user_name, u.binance_pay_id
			FROM global_share_wallets_log gswl
			LEFT JOIN users u ON u.id = CAST(gswl.user_id AS UNSIGNED)
			WHERE DATE(gswl.created_at) >= ? AND DATE(gswl.created_at) <= ?
			ORDER BY gswl.created_at DESC LIMIT ? OFFSET ?`, startDate, endDate, perPage, offset)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		defer rows.Close()

		var pools []map[string]interface{}
		for rows.Next() {
			var id uint64
			var userID, amount string
			var description models.NullString
			var createdAt models.NullTime
			var userName, binancePayID models.NullString
			if err := rows.Scan(&id, &userID, &amount, &description, &createdAt, &userName, &binancePayID); err != nil {
				continue
			}
			pools = append(pools, map[string]interface{}{
				"id": id, "user_id": userID, "amount": amount, "description": description.String,
				"created_at": createdAt.Time, "user_name": userName.String, "binance_pay_id": binancePayID.String,
			})
		}

		httpx.OK(w, map[string]interface{}{
			"status":     "success",
			"pools":      httpx.Paginate(pools, total, page, perPage),
			"start_date": startDate,
			"end_date":   endDate,
		})
	}
}

// --- POST /package-pools ---

func packagePoolStoreHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserID     *uint64  `json:"user_id"`
			PoolAmount *float64 `json:"pool_amount"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		errs := map[string][]string{}
		if body.UserID == nil {
			errs["user_id"] = []string{"The user id field is required."}
		} else {
			var exists int
			_ = d.DB.Get(&exists, "SELECT COUNT(*) FROM users WHERE id = ?", *body.UserID)
			if exists == 0 {
				errs["user_id"] = []string{"The selected user id is invalid."}
			}
		}
		if body.PoolAmount == nil || *body.PoolAmount < 0 {
			errs["pool_amount"] = []string{"The pool amount field is required and must be at least 0."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		// NOTE: the original hardcodes user_id=1, package_id=1 on every
		// insert regardless of the submitted (and validated!) user_id —
		// api_spec.md flags this as likely-a-bug-but-preserve-it. Preserved
		// verbatim here.
		if _, err := d.DB.Exec(`INSERT INTO package_pools (user_id, package_id, pool_amount, created_at, updated_at)
			VALUES (1, 1, ?, NOW(), NOW())`, *body.PoolAmount); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Pool added successfully."})
	}
}

// --- PUT /package-pools/{id} ---

func packagePoolUpdateHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(chi.URLParam(r, "id"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid package pool id")
			return
		}
		var exists int
		_ = d.DB.Get(&exists, "SELECT COUNT(*) FROM package_pools WHERE id = ?", id)
		if exists == 0 {
			httpx.Error(w, http.StatusNotFound, "Package pool not found")
			return
		}

		var body struct {
			PoolAmount *float64 `json:"pool_amount"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if body.PoolAmount == nil || *body.PoolAmount < 0 {
			httpx.ValidationError(w, map[string][]string{"pool_amount": {"The pool amount field is required and must be at least 0."}})
			return
		}

		if _, err := d.DB.Exec("UPDATE package_pools SET pool_amount = ?, updated_at = NOW() WHERE id = ?", *body.PoolAmount, id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Pool Values updated successfully."})
	}
}

// --- DELETE /package-pools/{id} ---

func packagePoolDestroyHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(chi.URLParam(r, "id"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid package pool id")
			return
		}
		var exists int
		_ = d.DB.Get(&exists, "SELECT COUNT(*) FROM package_pools WHERE id = ?", id)
		if exists == 0 {
			httpx.Error(w, http.StatusNotFound, "Package pool not found")
			return
		}
		if _, err := d.DB.Exec("DELETE FROM package_pools WHERE id = ?", id); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "success", "message": "Pool Values deleted successfully."})
	}
}

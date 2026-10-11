// mining_handler.go covers MiningController (api_spec.md "## MiningController").
package handlers

import (
	"database/sql"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
)

func RegisterMiningRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/mining/users", miningUsersHandler(d))
		r.Get("/api/v1/mining/search/{id}", miningSearchHandler(d))
		r.Post("/api/v1/mining/update/{id}", miningUpdateHandler(d))
	})

	// GET /mining/user/{userId} had NO auth middleware in the original at
	// all (see api_spec.md's cross-cutting note + BACKEND_CONVENTIONS.md's
	// disclosed fix policy) — mining data is sensitive enough to require a
	// session here, deliberately diverging from the original.
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Get("/api/v1/mining/user/{userId}", miningUserDataHandler(d))
	})
}

// miningUsersHandler ports MiningController@index — the original passes no
// data to the view at all (page is AJAX-driven via search/update below); we
// just confirm the route/role gate is satisfied.
func miningUsersHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpx.OK(w, map[string]interface{}{"success": true})
	}
}

func miningSearchHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(signetIDToNumeric(chi.URLParam(r, "id")))
		if !ok {
			httpx.OK(w, map[string]interface{}{"success": false, "message": "User not found"})
			return
		}

		var user models.User
		if err := d.DB.Get(&user, "SELECT * FROM users WHERE id = ?", id); err != nil {
			httpx.OK(w, map[string]interface{}{"success": false, "message": "User not found"})
			return
		}

		var userparentCount int
		_ = d.DB.Get(&userparentCount, "SELECT COUNT(*) FROM user_parents WHERE virtual_id = ? AND node IN ('active','gratitude')", id)

		var packageNames []string
		_ = d.DB.Select(&packageNames, `
			SELECT p.name FROM user_packages up
			JOIN packages p ON p.id = CAST(up.package AS UNSIGNED)
			WHERE up.user_id = ? ORDER BY up.created_at ASC`, id)
		var firstPackage, lastPackage interface{}
		if len(packageNames) > 0 {
			firstPackage = packageNames[0]
			lastPackage = packageNames[len(packageNames)-1]
		}

		var mining models.UserMining
		miningStatus := "inactive"
		var totalToken, dailyMining int64
		var miningToken float64
		if err := d.DB.Get(&mining, "SELECT * FROM user_minings WHERE user_id = ? LIMIT 1", id); err == nil {
			totalToken = mining.TotalToken
			miningToken = mining.MiningToken
			dailyMining = mining.DailyMining
			miningStatus = mining.Status
		}

		httpx.OK(w, map[string]interface{}{
			"success": true,
			"user": map[string]interface{}{
				"id":    models.SignetID(id),
				"name":  user.Name,
				"email": user.Email,
			},
			"packages": map[string]interface{}{"first": firstPackage, "last": lastPackage},
			"mining": map[string]interface{}{
				"total_token":  totalToken,
				"mining_token": miningToken,
				"daily_mining": dailyMining,
				"status":       miningStatus,
			},
			"sales": map[string]interface{}{"total_sales": userparentCount},
		})
	}
}

// miningUpdateHandler ports MiningController@update — the original reads
// $request->daily_mining / total_token / status directly with NO validate()
// call at all, so any type or absence is accepted as-is. Go requires a
// typed decode where PHP's dynamic property access would not; the pragmatic
// equivalent kept here is: a field present in the JSON body overwrites the
// mining row's value (best-effort numeric/string coercion, no error on a
// wrong type — silently ignored, matching "no validation"); a field absent
// leaves the existing row's value untouched (or defaults to zero-value on
// first insert, matching `new UserMining()`).
func miningUpdateHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUintParam(signetIDToNumeric(chi.URLParam(r, "id")))
		if !ok {
			httpx.OK(w, map[string]interface{}{"success": false, "message": "User not found"})
			return
		}
		var userCount int
		if d.Cfg.BlockchainEnabled {
			var integrated int
			if err := d.DB.Get(&integrated, "SELECT COUNT(*) FROM package_purchase_intents WHERE user_id=? AND purchase_kind='first' AND status='completed'", id); err != nil {
				httpx.Error(w, 503, "Vesting status temporarily unavailable")
				return
			}
			if integrated > 0 {
				httpx.Error(w, 409, "This user's vesting is managed by the blockchain vesting administration; legacy mining allocation cannot be changed")
				return
			}
		}
		_ = d.DB.Get(&userCount, "SELECT COUNT(*) FROM users WHERE id = ?", id)
		if userCount == 0 {
			httpx.OK(w, map[string]interface{}{"success": false, "message": "User not found"})
			return
		}

		var body map[string]interface{}
		_ = decodeJSON(r, &body) // no validate() call in the original — preserved

		var mining models.UserMining
		err := d.DB.Get(&mining, "SELECT * FROM user_minings WHERE user_id = ? LIMIT 1", id)
		found := err == nil
		if err != nil && err != sql.ErrNoRows {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if !found {
			mining = models.UserMining{Status: "inactive"}
		}
		mining.UserID = id
		if v, present := body["daily_mining"]; present {
			mining.DailyMining = int64(numFromAny(v))
		}
		if v, present := body["total_token"]; present {
			mining.TotalToken = int64(numFromAny(v))
		}
		if v, present := body["status"]; present {
			if s, ok := v.(string); ok {
				mining.Status = s
			}
		}

		if found {
			_, err = d.DB.Exec("UPDATE user_minings SET daily_mining = ?, total_token = ?, status = ?, updated_at = NOW() WHERE id = ?",
				mining.DailyMining, mining.TotalToken, mining.Status, mining.ID)
		} else {
			_, err = d.DB.Exec(`INSERT INTO user_minings (user_id, daily_mining, total_token, status, mining_token, created_at, updated_at)
				VALUES (?, ?, ?, ?, 0, NOW(), NOW())`, id, mining.DailyMining, mining.TotalToken, mining.Status)
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Could not update mining data")
			return
		}
		httpx.OK(w, map[string]interface{}{"success": true})
	}
}

func numFromAny(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

// miningUserDataHandler ports MiningController@getUserMiningData — a raw
// join of user_minings to users, 404 if no row. The original computes
// (mining_token/total_token)*100 with no zero-guard: PHP produces NAN and
// json_encode() then fails (Laravel throws a JsonEncodingException,
// surfacing as a 500), so returning 500 here on a total_token=0 row mirrors
// the original's actual observable behavior rather than silently emitting
// broken JSON (which Go's encoding/json cannot do for NaN/Inf either).
func miningUserDataHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := parseUintParam(chi.URLParam(r, "userId"))
		if !ok {
			httpx.JSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "Mining data not found"})
			return
		}

		var row struct {
			MiningToken float64         `db:"mining_token"`
			TotalToken  int64           `db:"total_token"`
			DailyMining int64           `db:"daily_mining"`
			Status      string          `db:"status"`
			UpdatedAt   models.NullTime `db:"updated_at"`
		}
		err := d.DB.Get(&row, `
			SELECT m.mining_token, m.total_token, m.daily_mining, m.status, m.updated_at
			FROM user_minings m JOIN users u ON u.id = m.user_id
			WHERE m.user_id = ? LIMIT 1`, userID)
		if err == sql.ErrNoRows {
			httpx.JSON(w, http.StatusNotFound, map[string]interface{}{"success": false, "error": "Mining data not found"})
			return
		}
		if err != nil {
			httpx.JSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": err.Error()})
			return
		}

		progress := (row.MiningToken / float64(row.TotalToken)) * 100
		if math.IsNaN(progress) || math.IsInf(progress, 0) {
			httpx.JSON(w, http.StatusInternalServerError, map[string]interface{}{"success": false, "error": "Division by zero"})
			return
		}

		httpx.OK(w, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"mining_token": miningRound(row.MiningToken, 8),
				"total_token":  row.TotalToken,
				"daily_mining": row.DailyMining,
				"status":       row.Status,
				"progress":     progress,
				"updated_at":   row.UpdatedAt.Time,
			},
		})
	}
}

func miningRound(v float64, places int) float64 {
	mult := math.Pow(10, float64(places))
	return math.Round(v*mult) / mult
}

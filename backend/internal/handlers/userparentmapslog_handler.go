// userparentmapslog_handler.go covers UserParentMapsLogController
// (api_spec.md "## UserParentMapsLogController").
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
)

func RegisterUserParentMapsLogRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/user-parent-logs", userParentLogsIndexHandler(d))
		r.Delete("/api/v1/user-parent-logs/{log}", userParentLogsDestroyHandler(d))
	})
}

// GET user-parent-logs (userparentlogs.index) — UserParentMapsLogController@index
//
// UserParentMapsLog::with('user')->whereHas('user', fn($q)=>$q->where('status','pending'))
//
//	->where('created_at','<=', now()->subHours(10))->orderBy('created_at','desc')->paginate(10)
func userParentLogsIndexHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage, offset := httpx.PageParams(r, 10)
		cutoff := time.Now().Add(-10 * time.Hour)

		var total int
		_ = d.DB.Get(&total, `
			SELECT COUNT(*) FROM user_parent_map_logs l
			JOIN users u ON u.id = l.user_id
			WHERE u.status = 'pending' AND l.created_at <= ?`, cutoff)

		type logRow struct {
			models.UserParentMapsLog
			UserName   models.NullString `db:"user_name"`
			UserEmail  models.NullString `db:"user_email"`
			UserStatus models.NullString `db:"user_status"`
			// "Activation Arrived User": User::where('id', $log->parent_id)->first()
			ParentUserID models.NullInt64  `db:"parent_user_id"`
			ParentName   models.NullString `db:"parent_name"`
		}
		var rows []logRow
		_ = d.DB.Select(&rows, `
			SELECT l.*, u.name AS user_name, u.email AS user_email, u.status AS user_status,
			       pu.id AS parent_user_id, pu.name AS parent_name
			FROM user_parent_map_logs l
			JOIN users u ON u.id = l.user_id
			LEFT JOIN users pu ON pu.id = l.parent_id
			WHERE u.status = 'pending' AND l.created_at <= ?
			ORDER BY l.created_at DESC
			LIMIT ? OFFSET ?`, cutoff, perPage, offset)

		logs := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			var userOut interface{}
			if row.UserID.Valid {
				userOut = map[string]interface{}{
					"id":        row.UserID.Int64,
					"signet_id": models.SignetID(uint64(row.UserID.Int64)),
					"name":      row.UserName.String,
					"email":     row.UserEmail.String,
					"status":    row.UserStatus.String,
				}
			}
			var parentOut interface{}
			if row.ParentUserID.Valid {
				parentOut = map[string]interface{}{
					"id":   row.ParentUserID.Int64,
					"name": row.ParentName.String,
				}
			}
			logs = append(logs, map[string]interface{}{
				"id":              row.ID,
				"user_id":         row.UserID,
				"parent_id":       row.ParentID,
				"created_row_ids": row.CreatedRowIDs.String,
				"note":            row.Note.String,
				"created_at":      row.CreatedAt,
				"user":            userOut,
				"parent_user":     parentOut,
			})
		}

		httpx.OK(w, map[string]interface{}{
			"status": "success",
			"logs":   httpx.Paginate(logs, total, page, perPage),
		})
	}
}

// DELETE user-parent-logs/{log} (userparentlogs.destroy) — UserParentMapsLogController@destroy
//
// Destructive, run inside DB::transaction with try/catch in the original:
//  1. lockForUpdate()->findOrFail($id)
//  2. delete the user's ReferralCode
//  3. delete all UserParent rows whose id is in $log->created_row_ids
//  4. delete the log row itself
//  5. hard-delete the User row
//
// Any failure rolls the whole thing back and returns the generic error
// flash; we mirror that by wrapping the whole sequence in one *sqlx.Tx and
// rolling back (via defer) on any error return.
func userParentLogsDestroyHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Cfg.RegistrationExpiryEnabled || d.Cfg.BlockchainEnabled {
			httpx.Error(w, http.StatusConflict, "Account deletion is disabled for automated or blockchain registrations; placement expiry preserves financial history")
			return
		}
		const genericErr = "Something went wrong while deleting records. No changes were saved."

		logID, ok := parseUintParam(chi.URLParam(r, "log"))
		if !ok {
			httpx.Error(w, http.StatusBadRequest, "Invalid log id")
			return
		}

		tx, err := d.DB.Beginx()
		if err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}
		defer tx.Rollback()

		var logRow models.UserParentMapsLog
		if err := tx.Get(&logRow, "SELECT * FROM user_parent_map_logs WHERE id = ? FOR UPDATE", logID); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}
		if !logRow.UserID.Valid {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}
		userID := uint64(logRow.UserID.Int64)

		if _, err := tx.Exec("DELETE FROM referral_codes WHERE user_id = ?", userID); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}

		var rowIDs []uint64
		if logRow.CreatedRowIDs.Valid && logRow.CreatedRowIDs.String != "" {
			if err := json.Unmarshal([]byte(logRow.CreatedRowIDs.String), &rowIDs); err != nil {
				httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
				return
			}
		}
		if len(rowIDs) > 0 {
			query, args, err := sqlx.In("DELETE FROM user_parents WHERE id IN (?)", rowIDs)
			if err != nil {
				httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
				return
			}
			query = tx.Rebind(query)
			if _, err := tx.Exec(query, args...); err != nil {
				httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
				return
			}
		}

		if _, err := tx.Exec("DELETE FROM user_parent_map_logs WHERE id = ?", logID); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}

		if _, err := tx.Exec("DELETE FROM users WHERE id = ?", userID); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}

		if err := tx.Commit(); err != nil {
			httpx.OK(w, map[string]interface{}{"status": "error", "message": genericErr})
			return
		}

		httpx.OK(w, map[string]interface{}{
			"status":  "success",
			"message": "Related UserParent records, referral code, user and log were deleted successfully.",
		})
	}
}

// roc_handler.go covers RocController (api_spec.md "## RocController").
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
	"signet-backend/internal/wallet"
)

func RegisterRocRoutes(r chi.Router, d *app.Deps) {
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Use(auth.RequireRole("company"))
		r.Get("/api/v1/roc", rocIncomeHandler(d))
		r.Post("/api/v1/roc/status-update", rocUpdateStatusHandler(d))
	})
}

// rocIncomeHandler ports RocController@rocIncome.
func rocIncomeHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var jobs []models.WeeklyPackageSummary
		_ = d.DB.Select(&jobs, "SELECT * FROM weekly_package_summaries ORDER BY id DESC")

		selectedJobID := r.URL.Query().Get("job_id")
		if selectedJobID == "" && len(jobs) > 0 {
			selectedJobID = jobs[0].JobID
		}
		if selectedJobID == "" {
			// Original: alert (mislabeled as success) + redirect back with
			// error flash "No ROC job found." — preserved as an error reply.
			httpx.OK(w, map[string]interface{}{"status": "error", "message": "No ROC job found."})
			return
		}

		var weeklySummary *models.WeeklyPackageSummary
		var ws models.WeeklyPackageSummary
		if err := d.DB.Get(&ws, "SELECT * FROM weekly_package_summaries WHERE job_id = ? LIMIT 1", selectedJobID); err == nil {
			weeklySummary = &ws
		}

		page, perPage, offset := httpx.PageParams(r, 10)
		var total int
		_ = d.DB.Get(&total, "SELECT COUNT(*) FROM roc_income_log WHERE job_id = ?", selectedJobID)

		type rocLogRow struct {
			models.RocIncomeLog
			UserName       models.NullString `db:"user_name" json:"user_name"`
			UserEmail      models.NullString `db:"user_email" json:"user_email"`
			BinancePayID   models.NullString `db:"binance_pay_id" json:"binance_pay_id"`
			WhatsappNumber models.NullString `db:"whatsapp_number" json:"whatsapp_number"`
		}
		var logs []rocLogRow
		_ = d.DB.Select(&logs, `
			SELECT r.*, u.name AS user_name, u.email AS user_email,
			       u.binance_pay_id AS binance_pay_id, u.whatsapp_number AS whatsapp_number
			FROM roc_income_log r LEFT JOIN users u ON u.id = r.user_id
			WHERE r.job_id = ? ORDER BY r.created_at DESC LIMIT ? OFFSET ?`, selectedJobID, perPage, offset)

		httpx.OK(w, map[string]interface{}{
			"jobs":            jobs,
			"selected_job_id": selectedJobID,
			"weekly_summary":  weeklySummary,
			"roc_income_logs": httpx.Paginate(logs, total, page, perPage),
		})
	}
}

// rocWalletCreditDescription labels the earn_logs line written when an admin
// approves a ROC and it is credited to the wallet.
const rocWalletCreditDescription = "ROC Income → credited to wallet"

// rocUpdateStatusHandler ports RocController@updateRocStatus. On a
// pending->paid transition it calls wallet.UpdateWallet — the CAP-GATED
// credit path (WalletService::updateWallet in the original; see
// financial_engine.md §1 / BACKEND_CONVENTIONS.md).
//
// Deliberate deviations, both of which let the original credit one ROC
// twice:
//   - paid is final. Un-ticking (paid -> pending) never reversed the credit,
//     and re-ticking credited it again; now paid -> pending is refused.
//   - pending -> paid is claimed with a single conditional UPDATE, so two
//     concurrent clicks can't both see "not paid yet" and both credit.
func rocUpdateStatusHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID     uint64 `json:"id"`
			Status string `json:"status"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.ValidationError(w, map[string][]string{"id": {"The id field is required."}})
			return
		}

		errs := map[string][]string{}
		if body.ID == 0 {
			errs["id"] = []string{"The id field is required."}
		}
		if body.Status != "pending" && body.Status != "paid" {
			errs["status"] = []string{"The selected status is invalid."}
		}
		if len(errs) > 0 {
			httpx.ValidationError(w, errs)
			return
		}

		var roc models.RocIncomeLog
		if err := d.DB.Get(&roc, "SELECT * FROM roc_income_log WHERE id = ?", body.ID); err != nil {
			httpx.OK(w, map[string]interface{}{"success": false, "message": "Record not found."})
			return
		}

		if body.Status == "pending" {
			if roc.Status == "paid" {
				httpx.OK(w, map[string]interface{}{"success": false,
					"message": "This ROC is already paid and credited to the wallet; it can't be set back to pending."})
				return
			}
			httpx.OK(w, map[string]interface{}{"success": true, "message": "Status updated successfully."})
			return
		}

		// Claim the pending -> paid transition atomically: only the request
		// that actually flips the row credits the wallet.
		res, err := d.DB.Exec("UPDATE roc_income_log SET status = 'paid', updated_at = NOW() WHERE id = ? AND status <> 'paid'", roc.ID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "Database error")
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			httpx.OK(w, map[string]interface{}{"success": true, "message": "Already paid — not credited again."})
			return
		}

		// The original reused roc.Description ("ROC Income") here, so the
		// wallet credit looked like a second weekly ROC in the user's earn
		// log. Label it as what it is: the approved ROC moving to the wallet.
		if err := wallet.UpdateWallet(d.DB, roc.UserID, roc.Amount, rocWalletCreditDescription); err != nil {
			// Release the claim so the admin can retry; nothing was credited.
			_, _ = d.DB.Exec("UPDATE roc_income_log SET status = ?, updated_at = NOW() WHERE id = ?", roc.Status, roc.ID)
			httpx.Error(w, http.StatusInternalServerError, "Could not credit wallet: "+err.Error())
			return
		}

		if roc.EarnLogID != 0 {
			_, _ = d.DB.Exec("UPDATE earn_logs SET description = 'ROC Income paid', updated_at = NOW() WHERE id = ?", roc.EarnLogID)
		}

		httpx.OK(w, map[string]interface{}{"success": true, "message": "Status updated successfully."})
	}
}

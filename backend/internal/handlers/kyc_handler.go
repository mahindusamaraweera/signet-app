package handlers

import (
	"database/sql"
	"net/http"

	"github.com/go-chi/chi/v5"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/models"
)

// KYC is now a read-only view of the authenticated user's connected Tier account.
// Historical records/files are retained; upload and local approval are retired.
func RegisterKycRoutes(r chi.Router, d *app.Deps) {
	r.Get("/storage/kyc/*", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	r.Group(func(r chi.Router) {
		r.Use(d.Auth.RequireAuth)
		r.Get("/api/v1/kyc", linkedTierKycHandler(d))
		r.Get("/api/v1/kyc/show", linkedTierKycHandler(d))
		for _, path := range []string{"/api/v1/kyc/create", "/api/v1/kyc/verified", "/api/v1/kyc/{id}/edit"} {
			r.Get(path, retiredKycHandler)
		}
		r.Post("/api/v1/kyc", retiredKycHandler)
		r.Put("/api/v1/kyc/{id}", retiredKycHandler)
		r.Delete("/api/v1/kyc/{id}", retiredKycHandler)
		r.Post("/api/v1/kyc/{id}/verify", retiredKycHandler)
		r.Post("/api/v1/kyc/{id}/unverify", retiredKycHandler)
	})
}
func retiredKycHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, http.StatusGone, map[string]string{"code": "kyc_method_retired", "message": "Identity verification is managed through your connected Tier account. Open Identity verification to continue."})
}
func linkedTierKycHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, private")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.Blockchain == nil {
			httpx.Error(w, 503, "Tier identity verification is temporarily unavailable")
			return
		}
		var localTierID string
		err := d.DB.Get(&localTierID, "SELECT tier_user_id FROM tier_account_links WHERE user_id=?", user.ID)
		if err == sql.ErrNoRows {
			httpx.JSON(w, 200, map[string]interface{}{"linked": false, "verified": false, "status": "not_linked", "telegram_links": []string{}})
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Account connection could not be checked")
			return
		}
		state, err := syncTierIdentity(r.Context(), d, user.ID)
		if err != nil || state.TierUserID == nil || !*state.Linked || *state.TierUserID != localTierID {
			httpx.Error(w, 503, "Tier verification could not be confirmed. Refresh shortly or check your Tier account connection.")
			return
		}
		// Preserve package community links, gated by the new authoritative approval.
		links := []string{}
		if *state.Verified {
			var rows []models.NullString
			err := d.DB.Select(&rows, `SELECT p.telegram_link FROM user_packages up LEFT JOIN packages p ON p.id = CAST(up.package AS UNSIGNED) WHERE up.user_id = ? AND up.status = 'active' ORDER BY up.id`, user.ID)
			if err == nil {
				for _, row := range rows {
					if row.Valid {
						links = append(links, row.String)
					}
				}
			}
		}
		httpx.JSON(w, 200, map[string]interface{}{"linked": *state.Linked, "verified": *state.Verified, "status": state.Status,
			"withdrawal_required": *state.WithdrawalRequired, "environment": state.Environment, "level": state.Level,
			"checked_at": state.CheckedAt, "valid_until": state.ValidUntil, "verification_url": state.VerificationURL, "telegram_links": links})
	}
}

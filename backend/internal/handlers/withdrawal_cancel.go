package handlers

import (
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"time"
)

func withdrawalCancelHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.Cfg == nil || !d.Cfg.BlockchainEnabled || !d.Cfg.BlockchainWithdrawalCancellationEnabled {
			httpx.Error(w, 404, "Cancellation is not enabled")
			return
		}
		if _, err := d.Cfg.WithdrawalAuthorizationKey(); err != nil || d.WithdrawalSecurity == nil || d.Blockchain == nil {
			httpx.Error(w, 503, "Cancellation unavailable")
			return
		}
		var body struct {
			ID       string `json:"id"`
			Password string `json:"password"`
		}
		if decodeJSON(r, &body) != nil || body.Password == "" || len(body.Password) > 72 {
			httpx.Error(w, 422, "Invalid cancellation request")
			return
		}
		if err := d.WithdrawalSecurity.RequestWithdrawalCancellation(r.Context(), user.ID, body.Password, body.ID, time.Now().UTC()); err != nil {
			httpx.Error(w, 409, "Cancellation could not be requested; check password and request status")
			return
		}
		httpx.JSON(w, 202, map[string]string{"id": body.ID, "status": "cancellation_requested", "message": "Check request status for confirmation. A submitted payout may require reconciliation instead of cancellation."})
	}
}

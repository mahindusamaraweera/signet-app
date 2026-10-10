package handlers

import (
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"time"
)

func withdrawalRequestsHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal request status unavailable")
			return
		}
		rows, err := d.WithdrawalSecurity.ListWithdrawalRequests(r.Context(), user.ID, time.Now().UTC())
		if err != nil {
			httpx.Error(w, 503, "Withdrawal request status unavailable")
			return
		}
		httpx.JSON(w, 200, map[string]interface{}{"requests": rows})
	}
}

package handlers

import (
	"errors"
	"net/http"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/totp"
	"signet-backend/internal/withdrawalsecurity"
)

func withdrawalAuthenticatorHandler(d *app.Deps, confirm bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal security setup is temporarily unavailable")
			return
		}
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		var body struct {
			Password  string `json:"password"`
			EmailID   string `json:"challenge_id"`
			EmailCode string `json:"email_code"`
			Code      string `json:"code"`
		}
		if err := decodeJSON(r, &body); err != nil || body.Password == "" || len(body.Password) > 72 {
			httpx.Error(w, 422, "Current password is required")
			return
		}
		var err error
		var setup withdrawalsecurity.AuthenticatorSetup
		if confirm {
			err = d.WithdrawalSecurity.ConfirmAuthenticator(r.Context(), user.ID, body.Password, body.Code, time.Now().UTC())
		} else {
			setup, err = d.WithdrawalSecurity.BeginAuthenticator(r.Context(), user.ID, body.Password, body.EmailID, body.EmailCode, time.Now().UTC())
		}
		if errors.Is(err, withdrawalsecurity.ErrChallengeCooldown) {
			httpx.Error(w, 429, "Please wait before starting another setup")
			return
		}
		if errors.Is(err, withdrawalsecurity.ErrAuthenticatorEnrollment) || errors.Is(err, withdrawalsecurity.ErrSecurityCredentials) || errors.Is(err, withdrawalsecurity.ErrInvalidChallenge) || errors.Is(err, totp.ErrInvalidOrReusedCode) {
			httpx.Error(w, 422, "Security proof is invalid, expired, reused or ineligible. Confirmed authenticators cannot be reset here.")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Authenticator setup temporarily unavailable")
			return
		}
		if confirm {
			httpx.OK(w, map[string]interface{}{"status": "confirmed", "message": "Authenticator confirmed. A 48-hour withdrawal security hold applies; a longer existing hold is preserved."})
		} else {
			httpx.JSON(w, 200, setup)
		}
	}
}

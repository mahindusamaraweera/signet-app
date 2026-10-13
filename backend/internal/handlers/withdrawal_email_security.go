package handlers

import (
	"errors"
	"net/http"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/withdrawalsecurity"
)

func withdrawalEmailIssueHandler(d *app.Deps) http.HandlerFunc {
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
			Password string `json:"password"`
		}
		if err := decodeJSON(r, &body); err != nil || body.Password == "" || len(body.Password) > 72 {
			httpx.Error(w, 422, "Current password is required")
			return
		}
		id, err := d.WithdrawalSecurity.IssueEmail(r.Context(), user.ID, body.Password, time.Now().UTC(), securitymail.New(d.Cfg))
		if errors.Is(err, withdrawalsecurity.ErrChallengeCooldown) {
			httpx.Error(w, 429, "Please wait before requesting another email code")
			return
		}
		if errors.Is(err, withdrawalsecurity.ErrSecurityCredentials) {
			httpx.Error(w, 422, "Password or security state is not eligible")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Verification email could not be delivered")
			return
		}
		httpx.JSON(w, 202, map[string]interface{}{"challenge_id": id, "message": "Verification code sent to your account email. It expires in 10 minutes."})
	}
}

func withdrawalEmailVerifyHandler(d *app.Deps) http.HandlerFunc {
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
			ID   string `json:"challenge_id"`
			Code string `json:"code"`
		}
		if err := decodeJSON(r, &body); err != nil {
			httpx.Error(w, 422, "Invalid verification request")
			return
		}
		err := d.WithdrawalSecurity.VerifyEmail(r.Context(), user.ID, body.ID, body.Code, time.Now().UTC())
		if errors.Is(err, withdrawalsecurity.ErrInvalidChallenge) {
			httpx.Error(w, 422, "Code is invalid, expired or exhausted. Request another code.")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Email verification temporarily unavailable")
			return
		}
		httpx.OK(w, map[string]interface{}{"status": "verified", "message": "Your current account email is verified for withdrawal security."})
	}
}

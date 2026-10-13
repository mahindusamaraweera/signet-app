package handlers

import (
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/withdrawalsecurity"
	"time"
)

func withdrawalRequestHandler(d *app.Deps, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.Cfg == nil || !d.Cfg.BlockchainEnabled || !d.Cfg.BlockchainWithdrawalSubmissionEnabled {
			httpx.Error(w, 404, "Withdrawal submission is not enabled")
			return
		}
		if _, err := d.Cfg.WithdrawalAuthorizationKey(); err != nil || d.WithdrawalSecurity == nil || d.Blockchain == nil {
			httpx.Error(w, 503, "Withdrawal service unavailable")
			return
		}
		var body struct {
			Password          string                             `json:"password"`
			AddressID         string                             `json:"address_id"`
			Amount            string                             `json:"amount"`
			ID                string                             `json:"id"`
			Proof             string                             `json:"proof"`
			Terms             withdrawalsecurity.WithdrawalTerms `json:"terms"`
			ChallengeID       string                             `json:"challenge_id"`
			EmailCode         string                             `json:"email_code"`
			AuthenticatorCode string                             `json:"authenticator_code"`
		}
		if decodeJSON(r, &body) != nil || body.Password == "" || len(body.Password) > 72 {
			httpx.Error(w, 422, "Invalid withdrawal request")
			return
		}
		now := time.Now().UTC()
		if action == "prepare" {
			prepared, err := d.WithdrawalSecurity.PrepareWithdrawal(r.Context(), user.ID, body.Password, body.AddressID, body.Amount, now)
			if err != nil {
				httpx.Error(w, 409, "Withdrawal security, address or waiting period requires attention")
				return
			}
			httpx.JSON(w, 200, prepared)
			return
		}
		if body.Terms.UserID != user.ID {
			httpx.Error(w, 422, "Invalid withdrawal identity")
			return
		}
		switch action {
		case "email":
			id, err := d.WithdrawalSecurity.IssueWithdrawalEmail(r.Context(), user.ID, body.Password, body.ID, body.Proof, body.Terms, now, securitymail.New(d.Cfg))
			if err != nil {
				httpx.Error(w, 409, "Verification unavailable; wait before retrying or prepare a new request")
				return
			}
			httpx.JSON(w, 200, map[string]string{"challenge_id": id})
			return
		case "confirm":
			if err := d.WithdrawalSecurity.ConfirmWithdrawal(r.Context(), user.ID, body.Password, body.ID, body.Proof, body.Terms, body.ChallengeID, body.EmailCode, body.AuthenticatorCode, now); err != nil {
				httpx.Error(w, 409, "Verification failed or request expired; check codes and security settings")
				return
			}
			httpx.JSON(w, 200, map[string]string{"status": "authorized", "id": body.ID})
			return
		case "queue":
			if err := d.WithdrawalSecurity.QueueWithdrawal(r.Context(), user.ID, body.Password, body.ID, body.Proof, body.Terms, now); err != nil {
				httpx.Error(w, 409, "Withdrawal could not be queued; check request status before retrying")
				return
			}
			httpx.JSON(w, 202, map[string]string{"status": "pending", "id": body.ID})
			return
		default:
			httpx.Error(w, 404, "Not found")
		}
	}
}

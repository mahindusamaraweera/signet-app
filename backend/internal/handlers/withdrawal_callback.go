package handlers

import (
	"crypto/sha256"
	"crypto/subtle"
	"github.com/go-chi/chi/v5"
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/httpx"
	"signet-backend/internal/withdrawalsecurity"
	"strconv"
	"time"
)

func RegisterWithdrawalCallbackRoutes(r chi.Router, d *app.Deps) {
	r.With(httpx.SecurityRateLimit(120, time.Minute)).Post("/api/v1/integration/withdrawals/revalidate", withdrawalCallbackHandler(d))
}
func withdrawalCallbackHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if d.Cfg == nil || !d.Cfg.BlockchainEnabled || !d.Cfg.BlockchainWithdrawalSubmissionEnabled {
			httpx.Error(w, 404, "Not found")
			return
		}
		token := d.Cfg.BlockchainWithdrawalCallbackToken
		key, err := d.Cfg.WithdrawalAuthorizationKey()
		if err != nil || len(token) < 32 || token == string(key) || token == d.Cfg.SessionSecret || token == d.Cfg.BlockchainServiceToken {
			httpx.Error(w, 503, "Withdrawal callback unavailable")
			return
		}
		expected := sha256.Sum256([]byte("Bearer " + token))
		provided := sha256.Sum256([]byte(r.Header.Get("Authorization")))
		if subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
			httpx.Error(w, 401, "Service authentication required")
			return
		}
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal callback unavailable")
			return
		}
		var body struct {
			AuthorizationID string                             `json:"authorization_id"`
			Terms           withdrawalsecurity.WithdrawalTerms `json:"terms"`
			RemoteID        string                             `json:"remote_withdrawal_id"`
			Nonce           string                             `json:"nonce"`
		}
		if decodeJSON(r, &body) != nil {
			httpx.Error(w, 422, "Invalid revalidation request")
			return
		}
		if d.Blockchain != nil && body.Terms.UserUUID != "" {
			var owner uint64
			if err := d.DB.GetContext(r.Context(), &owner, "SELECT id FROM users WHERE uuid=?", body.Terms.UserUUID); err != nil {
				httpx.Error(w, 422, "Invalid customer UUID")
				return
			}
			if body.Terms.UserID != 0 && body.Terms.UserID != owner {
				httpx.Error(w, 422, "Customer identity mismatch")
				return
			}
			body.Terms.UserID = owner
			body.Terms.UserUUID = "" // Local approval digests keep their internal foreign key.
		}
		remoteID, err := strconv.ParseUint(body.RemoteID, 10, 64)
		if err != nil || remoteID == 0 || strconv.FormatUint(remoteID, 10) != body.RemoteID {
			httpx.Error(w, 422, "Invalid revalidation request")
			return
		}
		proof, err := d.WithdrawalSecurity.BuildWithdrawalCurrentProof(r.Context(), body.AuthorizationID, body.Terms, remoteID, body.Nonce, key, time.Now().UTC())
		if err != nil {
			httpx.Error(w, 409, "Withdrawal approval is not currently eligible")
			return
		}
		httpx.JSON(w, 200, proof)
	}
}

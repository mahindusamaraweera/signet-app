package handlers

import (
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
)

func withdrawalSecurityStatusHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal security temporarily unavailable")
			return
		}
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		status, err := d.WithdrawalSecurity.Status(r.Context(), user.ID)
		if err != nil {
			httpx.Error(w, 503, "Withdrawal security status temporarily unavailable")
			return
		}
		if d.Cfg != nil && d.Cfg.BlockchainEnabled && d.Cfg.BlockchainWithdrawalSubmissionEnabled && d.Blockchain != nil {
			_, keyErr := d.Cfg.WithdrawalAuthorizationKey()
			token := d.Cfg.BlockchainWithdrawalCallbackToken
			status.SubmissionEnabled = keyErr == nil && len(token) >= 32 && token != d.Cfg.BlockchainWithdrawalAuthorizationKey && token != d.Cfg.SessionSecret && token != d.Cfg.BlockchainServiceToken
		}
		if d.Cfg != nil && d.Cfg.BlockchainEnabled && d.Cfg.BlockchainWithdrawalCancellationEnabled && d.Blockchain != nil {
			_, err := d.Cfg.WithdrawalAuthorizationKey()
			status.CancellationEnabled = err == nil
		}
		if status.SubmissionEnabled {
			if admission, err := d.Blockchain.WithdrawalAdmission(r.Context(), user.ID); err == nil {
				status.KycRequired = admission.KycRequired != nil && *admission.KycRequired
				status.KycVerified = admission.KycVerified != nil && *admission.KycVerified
				status.AdmissionChecked = true
				status.ActiveWithdrawalID = admission.ActiveWithdrawalID
				status.CooldownMinutes = *admission.CooldownMinutes
				status.CooldownUntil = admission.CooldownUntil
				status.ServerTime = admission.ServerTime
			}
		}
		httpx.JSON(w, 200, status)

	}
}

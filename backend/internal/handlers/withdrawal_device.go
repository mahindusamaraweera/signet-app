package handlers

import (
	"fmt"
	"net/http"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
)

func withdrawalDeviceCookie(userID uint64, credential string, secure bool) *http.Cookie {
	return &http.Cookie{Name: fmt.Sprintf("signet_withdrawal_device_%d", userID), Value: credential,
		Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 90 * 24 * 60 * 60}
}

func withdrawalDeviceRecordHandler(d *app.Deps) http.HandlerFunc {
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
		name := fmt.Sprintf("signet_withdrawal_device_%d", user.ID)
		credential := ""
		if cookie, err := r.Cookie(name); err == nil {
			credential = cookie.Value
		}
		device, credential, err := d.WithdrawalSecurity.RecordDevice(r.Context(), user.ID, credential, time.Now().UTC())
		if err != nil {
			httpx.Error(w, 503, "Device identification temporarily unavailable")
			return
		}
		http.SetCookie(w, withdrawalDeviceCookie(user.ID, credential, d.Cfg.AppEnv != "local"))
		httpx.JSON(w, 200, device)
	}
}

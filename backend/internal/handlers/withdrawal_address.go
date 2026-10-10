package handlers

import (
	"errors"
	"net/http"
	"time"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/totp"
	"signet-backend/internal/withdrawalsecurity"
)

func withdrawalAddressListHandler(d *app.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal security setup temporarily unavailable")
			return
		}
		addresses, err := d.WithdrawalSecurity.ListAddresses(r.Context(), user.ID)
		if err != nil {
			httpx.Error(w, 503, "Withdrawal addresses temporarily unavailable")
			return
		}
		httpx.JSON(w, 200, map[string]interface{}{"addresses": addresses})
	}
}

func withdrawalAddressHandler(d *app.Deps, confirm bool) http.HandlerFunc {
	return withdrawalAddressActionHandler(d, confirm, false)
}

func withdrawalAddressActionHandler(d *app.Deps, confirm, revoke bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := auth.UserFromContext(r.Context())
		if user == nil {
			httpx.Error(w, 401, "Authentication required")
			return
		}
		if d.WithdrawalSecurity == nil {
			httpx.Error(w, 503, "Withdrawal security setup temporarily unavailable")
			return
		}
		var body struct {
			Password    string `json:"password"`
			Address     string `json:"address"`
			ChallengeID string `json:"challenge_id"`
			EmailCode   string `json:"email_code"`
			Code        string `json:"code"`
		}
		addressLength := 42
		if revoke {
			addressLength = 64
		}
		if err := decodeJSON(r, &body); err != nil || body.Password == "" || len(body.Password) > 72 || len(body.Address) != addressLength || (confirm && (len(body.ChallengeID) != 64 || len(body.EmailCode) != 6 || len(body.Code) != 6)) {
			httpx.Error(w, 422, "Invalid address verification request")
			return
		}
		var err error
		var id string
		if revoke && confirm {
			err = d.WithdrawalSecurity.ConfirmAddressRevocation(r.Context(), user.ID, body.Password, body.Address, body.ChallengeID, body.EmailCode, body.Code, time.Now().UTC())
		} else if revoke {
			id, err = d.WithdrawalSecurity.IssueAddressRevocation(r.Context(), user.ID, body.Password, body.Address, time.Now().UTC(), securitymail.New(d.Cfg))
		} else if confirm {
			err = d.WithdrawalSecurity.ConfirmAddress(r.Context(), user.ID, body.Password, body.Address, body.ChallengeID, body.EmailCode, body.Code, time.Now().UTC())
		} else {
			id, err = d.WithdrawalSecurity.IssueAddress(r.Context(), user.ID, body.Password, body.Address, time.Now().UTC(), securitymail.New(d.Cfg))
		}
		if errors.Is(err, withdrawalsecurity.ErrChallengeCooldown) {
			httpx.Error(w, 429, "Please wait before requesting another address code")
			return
		}
		if errors.Is(err, withdrawalsecurity.ErrSecurityCredentials) || errors.Is(err, withdrawalsecurity.ErrInvalidChallenge) || errors.Is(err, withdrawalsecurity.ErrWithdrawalAddress) || errors.Is(err, totp.ErrInvalidOrReusedCode) {
			httpx.Error(w, 422, "Address or security proof is invalid, expired, reused or ineligible")
			return
		}
		if err != nil {
			httpx.Error(w, 503, "Address verification temporarily unavailable")
			return
		}
		if revoke && confirm {
			httpx.OK(w, map[string]interface{}{"status": "revoked", "message": "Withdrawal address revoked. Outstanding authorizations must be rechecked before payout."})
		} else if confirm {
			httpx.OK(w, map[string]interface{}{"status": "verified", "message": "Address verified. A 24-hour address waiting period and any existing security hold apply. Withdrawal submission is not enabled yet."})
		} else {
			httpx.OK(w, map[string]interface{}{"challenge_id": id, "message": "Destination-bound verification code sent to your account email. It expires in 10 minutes."})
		}
	}
}

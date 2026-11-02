package handlers

import (
	"errors"
	"log"
	"net/http"
	"signet-backend/internal/app"
	"signet-backend/internal/auth"
	"signet-backend/internal/httpx"
	"signet-backend/internal/securitymail"
	"signet-backend/internal/totp"
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
		if action == "prepare" || action == "queue" {
			admission, err := d.Blockchain.WithdrawalAdmission(r.Context(), user.ID)
			if err != nil {
				httpx.JSON(w, 503, map[string]string{"code": "withdrawal_admission_unavailable", "message": "Withdrawal status or cooldown could not be confirmed. Check status before trying again."})
				return
			}
			if admission.KycRequired != nil && *admission.KycRequired && (admission.KycVerified == nil || !*admission.KycVerified) {
				httpx.JSON(w, 409, map[string]string{"code": "kyc_required", "message": "Connect your Tier account and complete identity verification before withdrawing."})
				return
			}
			if !*admission.CanRequest {
				result := map[string]string{"code": "withdrawal_already_processing", "message": "An existing withdrawal is processing or needs review. Wait for completion or confirmed cancellation before starting another."}
				if admission.ActiveWithdrawalID != nil {
					result["active_withdrawal_id"] = *admission.ActiveWithdrawalID
				}
				if admission.ActiveWithdrawalID == nil && admission.CooldownUntil != nil {
					result["code"] = "withdrawal_cooldown_active"
					result["message"] = "The withdrawal cooldown is still active. Check the countdown before trying again."
					result["available_at"] = admission.CooldownUntil.UTC().Format(time.RFC3339)
				}
				httpx.JSON(w, 409, result)
				return
			}
		}
		// The remote policy read may take time. Never consume approval using
		// a timestamp captured before that network call.
		now = time.Now().UTC()
		if action == "prepare" {
			prepared, err := d.WithdrawalSecurity.PrepareWithdrawal(r.Context(), user.ID, body.Password, body.AddressID, body.Amount, now)
			if err != nil {
				code, message := withdrawalConfirmationError(err)
				log.Printf("withdrawal verification rejected: stage=%s user_id=%d code=%s", action, user.ID, code)
				httpx.JSON(w, 409, withdrawalErrorResponse(err, code, message))
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
				code, message := withdrawalConfirmationError(err)
				log.Printf("withdrawal verification rejected: stage=email user_id=%d code=%s", user.ID, code)
				httpx.JSON(w, 409, withdrawalErrorResponse(err, code, message))
				return
			}
			httpx.JSON(w, 200, map[string]string{"challenge_id": id})
			return
		case "confirm":
			if err := d.WithdrawalSecurity.ConfirmWithdrawal(r.Context(), user.ID, body.Password, body.ID, body.Proof, body.Terms, body.ChallengeID, body.EmailCode, body.AuthenticatorCode, now); err != nil {
				code, message := withdrawalConfirmationError(err)
				log.Printf("withdrawal verification rejected: stage=%s user_id=%d code=%s", action, user.ID, code)
				httpx.JSON(w, 409, withdrawalErrorResponse(err, code, message))
				return
			}
			httpx.JSON(w, 200, map[string]string{"status": "authorized", "id": body.ID})
			return
		case "queue":
			if err := d.WithdrawalSecurity.QueueWithdrawal(r.Context(), user.ID, body.Password, body.ID, body.Proof, body.Terms, now); err != nil {
				code, message := withdrawalConfirmationError(err)
				httpx.JSON(w, 409, withdrawalErrorResponse(err, code, message))
				return
			}
			httpx.JSON(w, 202, map[string]string{"status": "pending", "id": body.ID})
			return
		default:
			httpx.Error(w, 404, "Not found")
		}
	}
}

// Only fixed diagnostics are returned; database errors and secrets stay private.
func withdrawalConfirmationError(err error) (string, string) {
	var eligibility *withdrawalsecurity.EligibilityError
	if errors.As(err, &eligibility) {
		return eligibility.Code, eligibility.Message
	}
	switch {
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailDelivery):
		return "withdrawal_email_delivery_failed", "The verification email could not be sent. Wait before requesting a new email code; contact support if delivery keeps failing."
	case errors.Is(err, withdrawalsecurity.ErrChallengeCooldown):
		return "withdrawal_request_cooldown", "Too many recent requests. Wait before requesting another verification email. Email codes are limited to five per hour."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalExpired):
		return "withdrawal_request_expired", "This withdrawal request expired. Start a new withdrawal and use the new verification email."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailExpired):
		return "withdrawal_email_expired", "Your email code expired. Request a new code, or start again if the withdrawal request has expired."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailSuperseded):
		return "withdrawal_email_replaced", "A newer email code was sent. Use the latest verification email."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailAttempts):
		return "withdrawal_email_attempts_exhausted", "Too many incorrect verification attempts. Request a new email code."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailUsed):
		return "withdrawal_email_used", "This email code was already used. Check request status before trying again."
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalEmailIncorrect):
		return "withdrawal_email_incorrect", "The email code is incorrect. Enter the 6-digit code from your latest withdrawal verification email."
	case errors.Is(err, totp.ErrReusedCode):
		return "withdrawal_authenticator_reused", "This authenticator code was already used. Wait for the next code, then enter it with your email code."
	case errors.Is(err, totp.ErrInvalidCode):
		return "withdrawal_authenticator_invalid", "The authenticator code is incorrect or expired. Enter the current code from your withdrawal authenticator and check your phone's automatic time setting."
	default:
		return "withdrawal_verification_failed", "Verification could not complete. Check your password and withdrawal security settings, then check request status before retrying."
	}
}

func withdrawalErrorResponse(err error, code, message string) map[string]string {
	response := map[string]string{"message": message, "code": code}
	var eligibility *withdrawalsecurity.EligibilityError
	if errors.As(err, &eligibility) && eligibility.AvailableAt != "" {
		response["available_at"] = eligibility.AvailableAt
	}
	return response
}

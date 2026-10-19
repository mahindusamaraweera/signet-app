package handlers

import (
	"errors"
	"signet-backend/internal/totp"
	"signet-backend/internal/withdrawalsecurity"
	"testing"
)

func TestWithdrawalConfirmationDiagnostics(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{withdrawalsecurity.ErrWithdrawalEmailDelivery, "withdrawal_email_delivery_failed"},
		{withdrawalsecurity.ErrChallengeCooldown, "withdrawal_request_cooldown"},
		{withdrawalsecurity.ErrWithdrawalExpired, "withdrawal_request_expired"},
		{withdrawalsecurity.ErrWithdrawalEmailExpired, "withdrawal_email_expired"},
		{withdrawalsecurity.ErrWithdrawalEmailIncorrect, "withdrawal_email_incorrect"},
		{withdrawalsecurity.ErrWithdrawalEmailSuperseded, "withdrawal_email_replaced"},
		{withdrawalsecurity.ErrWithdrawalEmailAttempts, "withdrawal_email_attempts_exhausted"},
		{withdrawalsecurity.ErrWithdrawalEmailUsed, "withdrawal_email_used"},
		{totp.ErrInvalidCode, "withdrawal_authenticator_invalid"},
		{totp.ErrReusedCode, "withdrawal_authenticator_reused"},
		{errors.New("private database failure"), "withdrawal_verification_failed"},
	}
	for _, c := range cases {
		code, message := withdrawalConfirmationError(c.err)
		if code != c.code || message == "" || message == c.err.Error() {
			t.Fatalf("unsafe or incorrect diagnostic: %s", code)
		}
	}
}

func TestWithdrawalWaitResponseIncludesStructuredDeadline(t *testing.T) {
	err := &withdrawalsecurity.EligibilityError{Code: "withdrawal_security_wait", Message: "Waiting period active", AvailableAt: "2026-10-19T14:16:49Z"}
	code, message := withdrawalConfirmationError(err)
	response := withdrawalErrorResponse(err, code, message)
	if response["available_at"] != err.AvailableAt || response["code"] != err.Code {
		t.Fatal("missing structured wait deadline")
	}
	response = withdrawalErrorResponse(errors.New("private database failure"), "withdrawal_verification_failed", "Check status")
	if len(response) != 2 {
		t.Fatal("unexpected private diagnostic fields")
	}
}

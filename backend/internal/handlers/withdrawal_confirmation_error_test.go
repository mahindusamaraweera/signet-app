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

package handlers

import (
	"context"
	"errors"
	"github.com/go-sql-driver/mysql"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/withdrawalsecurity"
	"strings"
	"testing"
)

func TestWithdrawalDiagnosticNeverIncludesRawErrorsOrSQL(t *testing.T) {
	for _, tc := range []struct {
		err      error
		expected string
	}{
		{errors.New("signature=secret-key SQL values"), "stage=token_reservation code=unexpected_failure"},
		{withdrawalsecurity.ErrWithdrawalAuthorization, "stage=token_reservation code=source_authorization_invalid"},
		{context.DeadlineExceeded, "stage=token_reservation code=request_timeout"},
		{&mysql.MySQLError{Number: 1267, Message: "secret-key SQL values"}, "stage=token_reservation code=database_1267"},
		{&blockchain.RequestFailure{StatusCode: 422, Code: "withdrawal_signature_invalid"}, "stage=token_reservation http=422 code=withdrawal_signature_invalid"},
	} {
		result := withdrawalDiagnostic(withdrawalStage("token_reservation", tc.err))
		if result != tc.expected || strings.Contains(result, "secret-key") {
			t.Fatalf("unsafe diagnostic: %s", result)
		}
	}
	if withdrawalStage("record", nil) != nil {
		t.Fatal("success must remain success")
	}
	if !errors.Is(withdrawalStage("build", withdrawalsecurity.ErrWithdrawalAuthorization), withdrawalsecurity.ErrWithdrawalAuthorization) {
		t.Fatal("lost sentinel")
	}
}

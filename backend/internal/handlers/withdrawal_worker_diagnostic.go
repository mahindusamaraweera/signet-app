package handlers

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"signet-backend/internal/blockchain"
	"signet-backend/internal/withdrawalsecurity"
)

type withdrawalStageError struct {
	stage string
	cause error
}

func (e *withdrawalStageError) Error() string { return "withdrawal stage failed: " + e.stage }
func (e *withdrawalStageError) Unwrap() error { return e.cause }
func withdrawalStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &withdrawalStageError{stage: stage, cause: err}
}

// Never use the underlying Error() string: remote bodies and SQL may carry secrets.
func withdrawalDiagnostic(err error) string {
	stage := "load_terms"
	var staged *withdrawalStageError
	if errors.As(err, &staged) {
		stage = staged.stage
	}
	code := "unexpected_failure"
	var remote *blockchain.RequestFailure
	if errors.As(err, &remote) {
		return fmt.Sprintf("stage=%s http=%d code=%s", stage, remote.StatusCode, remote.Code)
	}
	var database *mysql.MySQLError
	if errors.As(err, &database) {
		return fmt.Sprintf("stage=%s code=database_%d", stage, database.Number)
	}
	switch {
	case errors.Is(err, withdrawalsecurity.ErrSecurityCredentials):
		code = "source_security_changed"
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalAddress):
		code = "source_address_ineligible"
	case errors.Is(err, withdrawalsecurity.ErrWithdrawalAuthorization):
		code = "source_authorization_invalid"
	case errors.Is(err, context.DeadlineExceeded):
		code = "request_timeout"
	case errors.Is(err, blockchain.ErrRejected):
		code = "gateway_terms_or_identity_rejected"
	case errors.Is(err, blockchain.ErrUnavailable):
		code = "gateway_unavailable_or_response_invalid"
	}
	return fmt.Sprintf("stage=%s code=%s", stage, code)
}

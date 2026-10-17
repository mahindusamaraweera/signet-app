package blockchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RequestFailure carries machine-readable diagnostics without granting retry authority.
type RequestFailure struct {
	StatusCode int
	Code       string
	cause      error
}

func (e *RequestFailure) Error() string { return e.cause.Error() }
func (e *RequestFailure) Unwrap() error { return e.cause }

func requestFailure(res *http.Response, sentinel error, method, path string) error {
	data, _ := io.ReadAll(io.LimitReader(res.Body, 300))
	var envelope struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(data, &envelope)
	code := "http_rejected"
	switch envelope.Code {
	case "withdrawal_signature_invalid", "withdrawal_key_unavailable", "withdrawal_assertion_time_invalid", "withdrawal_mapping_missing", "withdrawal_terms_conflict", "withdrawal_insufficient_balance", "withdrawal_custody_held", "withdrawal_cooldown_active", "withdrawal_limit_exceeded", "withdrawal_fee_destination_missing", "withdrawal_validation_or_configuration_failed":
		code = envelope.Code
	}
	snippet := strings.Join(strings.Fields(string(data)), " ")
	if snippet == "" {
		snippet = "(empty body)"
	}
	return &RequestFailure{StatusCode: res.StatusCode, Code: code, cause: fmt.Errorf("%w: %s %s: HTTP %d: %s", sentinel, method, path, res.StatusCode, snippet)}
}

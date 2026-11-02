package blockchain

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
)

// DepositFailureDiagnostic never logs response bodies, URLs or credentials.
// RequestFailure.Error contains a remote body and must not be logged here.
func DepositFailureDiagnostic(err error) string {
	var failure *RequestFailure
	if errors.As(err, &failure) {
		reason := "http_rejected"
		switch failure.StatusCode {
		case 401:
			reason = "authentication_rejected"
		case 403:
			reason = "access_or_environment_rejected"
		case 404:
			reason = "custody_mapping_or_route_missing"
		case 409:
			reason = "identity_or_custody_conflict"
		case 422:
			reason = "request_validation_rejected"
		case 429:
			reason = "rate_limited"
		}
		return fmt.Sprintf("stage=token_deposits http=%d code=%s reason=%s", failure.StatusCode, failure.Code, reason)
	}
	reason := "request_unavailable"
	var dns *net.DNSError
	var network net.Error
	var certificate *tls.CertificateVerificationError
	var syntax *json.SyntaxError
	switch {
	case errors.As(err, &dns):
		reason = "dns_lookup_failed"
	case errors.Is(err, context.DeadlineExceeded):
		reason = "request_timeout"
	case errors.Is(err, context.Canceled):
		reason = "request_canceled"
	case errors.As(err, &certificate):
		reason = "tls_certificate_failed"
	case errors.As(err, &network):
		reason = "network_connection_failed"
		if network.Timeout() {
			reason = "request_timeout"
		}
	case strings.Contains(err.Error(), "missing/invalid UUID for local user "):
		reason = "customer_uuid_missing_or_invalid"
	case errors.As(err, &syntax), strings.Contains(err.Error(), "invalid JSON response:"):
		reason = "invalid_json_response"
	case strings.Contains(err.Error(), "response larger than 64 KiB"):
		reason = "response_too_large"
	case errors.Is(err, ErrRejected):
		reason = "identity_or_request_rejected"
	}
	return "stage=token_deposits http=0 reason=" + reason
}

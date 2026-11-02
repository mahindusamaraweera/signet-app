package blockchain

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

func TestDepositDiagnosticsPreserveCauseWithoutRemoteSecrets(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&RequestFailure{StatusCode: 404, Code: "http_rejected", cause: errors.New("secret-token\ninjected")}, "http=404 code=http_rejected reason=custody_mapping_or_route_missing"},
		{fmt.Errorf("%w: %w", ErrUnavailable, context.DeadlineExceeded), "reason=request_timeout"},
		{fmt.Errorf("%w: %w", ErrUnavailable, &net.DNSError{Err: "secret-token", Name: "private-host"}), "reason=dns_lookup_failed"},
		{fmt.Errorf("%w: missing/invalid UUID for local user 42", ErrUnavailable), "reason=customer_uuid_missing_or_invalid"},
		{fmt.Errorf("%w: invalid JSON response: secret-token", ErrUnavailable), "reason=invalid_json_response"},
	} {
		diagnostic := DepositFailureDiagnostic(tc.err)
		if !strings.Contains(diagnostic, tc.want) || strings.Contains(diagnostic, "secret") || strings.Contains(diagnostic, "\n") {
			t.Fatal(diagnostic)
		}
	}
}

package blockchain

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRequestFailureUsesOnlyWhitelistedCodesAndRetainsSentinel(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"code":"withdrawal_signature_invalid","message":"secret assertion"}`, "withdrawal_signature_invalid"},
		{`{"code":"secret assertion","message":"secret assertion"}`, "http_rejected"},
		{`not json secret assertion`, "http_rejected"},
	} {
		err := requestFailure(&http.Response{StatusCode: 422, Body: io.NopCloser(strings.NewReader(tc.body))}, ErrRejected, "POST", "/api/v1/integration/withdrawals/reserve")
		var failure *RequestFailure
		if !errors.Is(err, ErrRejected) || !errors.As(err, &failure) || failure.Code != tc.code || failure.StatusCode != 422 {
			t.Fatalf("wrong failure classification")
		}
	}
}

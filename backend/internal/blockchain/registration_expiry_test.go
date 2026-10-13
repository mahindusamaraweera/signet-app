package blockchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegistrationExpirySealRequiresExactDurableEvidence(t *testing.T) {
	for _, tc := range []struct {
		body          string
		valid, sealed bool
	}{
		{`{"can_expire":false,"seal":null}`, true, false},
		{`{"can_expire":true,"seal":{"placement_log_id":"7","external_user_id":"42","sealed_at":"2026-01-01T00:00:00Z","scanned_block":990,"observed_head":1000}}`, true, true},
		{`{"can_expire":true}`, false, false},
		{`{"seal":null}`, false, false},
		{`{"can_expire":true,"seal":{"placement_log_id":"8","external_user_id":"42","sealed_at":"2026-01-01T00:00:00Z","scanned_block":990,"observed_head":1000}}`, false, false},
		{`{"can_expire":false,"seal":{}}`, false, false},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/integration/registration-expiry/seal" || r.Header.Get("Idempotency-Key") != "signet-app:expiry-seal:42:7" || r.Header.Get("Authorization") != "Bearer test" {
				t.Error("incorrect seal transport")
			}
			w.Write([]byte(tc.body))
		}))
		g, _ := New(s.URL, "test")
		seal, err := g.SealRegistrationExpiry(context.Background(), 42, 7)
		s.Close()
		if (err == nil) != tc.valid || (seal != nil) != tc.sealed {
			t.Fatalf("invalid clearance accepted: %s: %v", tc.body, err)
		}
	}
}

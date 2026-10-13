package blockchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplacementTransportConfirmsExactCommittedIdentities(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"reopened":true,"external_user_id":"42","expired_placement_log_id":"7","new_placement_log_id":"8","external_purchase_id":"2"}`, true},
		{`{"reopened":true,"external_user_id":"43","expired_placement_log_id":"7","new_placement_log_id":"8","external_purchase_id":"2"}`, false},
		{`{"reopened":true,"external_user_id":"42","expired_placement_log_id":"7","new_placement_log_id":"9","external_purchase_id":"2"}`, false},
		{`{"reopened":false,"external_user_id":"42","expired_placement_log_id":"7","new_placement_log_id":"8","external_purchase_id":"2"}`, false},
		{`{}`, false},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/integration/registration-expiry/reopen" || r.Header.Get("Idempotency-Key") != "signet-app:registration-reopen:42:7:8:2" || r.Header.Get("Authorization") != "Bearer test" {
				t.Error("wrong replacement transport")
			}
			w.Write([]byte(tc.body))
		}))
		g, _ := New(s.URL, "test")
		err := g.ReopenRegistration(context.Background(), 42, 7, 8, 2)
		s.Close()
		if (err == nil) != tc.valid {
			t.Fatalf("wrong replacement confirmation: %v", err)
		}
	}
}

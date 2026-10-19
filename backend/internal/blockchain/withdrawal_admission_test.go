package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithdrawalAdmissionRequiresExplicitConsistentAuthority(t *testing.T) {
	cases := []struct {
		body  string
		valid bool
	}{
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":30,"cooldown_until":null,"active_withdrawal_id":null,"can_request":true}`, true},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":30,"cooldown_until":"2026-10-19T10:20:00Z","active_withdrawal_id":null,"can_request":false}`, true},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"cooldown_until":null,"active_withdrawal_id":"508","can_request":false}`, true},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"active_withdrawal_id":"508","can_request":true}`, false},
		{`{"can_request":true}`, false},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":-1,"can_request":true}`, false},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"can_request":false}`, false},
	}
	for _, c := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.URL.Path != "/api/v1/integration/withdrawals/admission" || body["external_user_id"] != "42" || r.Header.Get("Authorization") != "Bearer test" {
				t.Error("wrong owner or transport")
			}
			_, _ = w.Write([]byte(c.body))
		}))
		g, _ := New(server.URL, "test")
		_, err := g.WithdrawalAdmission(context.Background(), 42)
		server.Close()
		if (err == nil) != c.valid {
			t.Fatalf("bad policy validation for %s: %v", c.body, err)
		}
	}
}

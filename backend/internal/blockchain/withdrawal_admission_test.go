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
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":30,"cooldown_until":null,"active_withdrawal_id":null,"can_request":true}`, true},
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":30,"cooldown_until":"2026-10-19T10:20:00Z","active_withdrawal_id":null,"can_request":false}`, true},
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"cooldown_until":null,"active_withdrawal_id":"508","can_request":false}`, true},
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"active_withdrawal_id":"508","can_request":true}`, false},
		{`{"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"can_request":true}`, false},
		{`{"can_request":true}`, false},
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":-1,"can_request":true}`, false},
		{`{"kyc_required":false,"kyc_verified":false,"server_time":"2026-10-19T10:00:00Z","cooldown_minutes":0,"can_request":false}`, false},
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

func TestWithdrawalAdmissionAcceptsAuthoritativeKycBlock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"server_time":"2026-10-24T00:00:00Z","cooldown_minutes":30,"cooldown_until":null,"active_withdrawal_id":null,"can_request":false,"kyc_required":true,"kyc_verified":false}`))
	}))
	defer server.Close()
	gateway, err := New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	state, err := gateway.WithdrawalAdmission(context.Background(), 42)
	if err != nil || *state.CanRequest || state.KycRequired == nil || !*state.KycRequired {
		t.Fatalf("KYC admission not recognized: %v", err)
	}
}

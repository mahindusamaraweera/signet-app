package blockchain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnsureWalletIdentityAndIdempotency(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integration/wallets" || r.Header.Get("Idempotency-Key") != "signet-app:wallet:42" || r.Header.Get("Authorization") != "Bearer test" {
			t.Error("incorrect gateway request")
		}
		w.Write([]byte(`{"address":"0x1234567890123456789012345678901234567890","chain_id":97,"asset":"USDT","available":"0","provisioning_status":"ready"}`))
	}))
	defer s.Close()
	g, err := New(s.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	w, err := g.EnsureWallet(context.Background(), 42)
	if err != nil || w.ChainID != 97 {
		t.Fatalf("wallet result: %v", err)
	}
}

func TestRejectInvalidWallet(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"address":"0xBAD","chain_id":97,"asset":"USDT","provisioning_status":"ready"}`))
	}))
	defer s.Close()
	g, _ := New(s.URL, "test")
	_, err := g.EnsureWallet(context.Background(), 42)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestRejectInsecureService(t *testing.T) {
	for _, u := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?token=x"} {
		if _, err := New(u, "test"); err == nil {
			t.Errorf("accepted %s", u)
		}
	}
}

func TestRejectedAdjustmentIsDistinctFromUncertainFailure(t *testing.T) {
	for _, status := range []int{422, 500, 503} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }))
		g, _ := New(s.URL, "test")
		err := g.request(context.Background(), http.MethodPost, "/vesting/adjust", "key", map[string]string{}, &map[string]string{})
		if errors.Is(err, ErrRejected) != (status == 422) {
			t.Fatalf("incorrect classification for HTTP %d: %v", status, err)
		}
		s.Close()
	}
}

func TestExpiryRequiresExplicitEligibility(t *testing.T) {
	for _, tc := range []struct {
		body    string
		allowed bool
		fails   bool
	}{
		{`{"can_expire":true}`, true, false},
		{`{"can_expire":false}`, false, false},
		{`{}`, false, true},
		{`{"can_expire":null}`, false, true},
		{`{"can_expire":"true"}`, false, true},
	} {
		t.Run(tc.body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.body)) }))
			defer s.Close()
			g, _ := New(s.URL, "test")
			allowed, err := g.CanExpireRegistration(context.Background(), 42)
			if allowed != tc.allowed || (err != nil) != tc.fails {
				t.Fatalf("allowed=%v err=%v", allowed, err)
			}
		})
	}
}

func TestEnvironmentIsSentOnCustodyRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Signet-App-Environment") != "production" {
			t.Error("missing environment boundary")
		}
		w.Write([]byte(`{"address":"0x1234567890123456789012345678901234567890","chain_id":56,"asset":"USDT","available":"0","provisioning_status":"ready"}`))
	}))
	defer server.Close()
	gateway, err := New(server.URL, "test", "production")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gateway.EnsureWallet(context.Background(), 461); err != nil {
		t.Fatal(err)
	}
	if _, err := New(server.URL, "test", "production\r\nX-Injected: value"); err == nil {
		t.Fatal("invalid environment accepted")
	}
}

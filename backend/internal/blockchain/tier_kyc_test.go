package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func kycBool(value bool) *bool { return &value }
func validTierKyc() TierKycStatus {
	id := "30"
	path := "/identity-verification?account=11111111-1111-4111-8111-111111111111"
	until := time.Now().UTC().Add(30 * time.Minute)
	return TierKycStatus{ExternalUserID: "42", Linked: kycBool(true), TierUserID: &id, Status: "approved", Verified: kycBool(true), WithdrawalRequired: kycBool(true), Environment: "production", Level: "SIGNET", CheckedAt: time.Now().UTC(), ValidUntil: &until, VerificationPath: &path}
}
func TestTierKycIsAuthenticatedMetadataOnlyAndNoVestingOptionIsNeeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/integration/tier-link/kyc" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer integration-test" {
			t.Error("incorrect integration request")
		}
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["external_user_id"] != "42" || body["source"] != "signet-app" || len(body) != 2 {
			t.Error("identity not derived from caller")
		}
		json.NewEncoder(w).Encode(validTierKyc())
	}))
	defer server.Close()
	gateway, err := New(server.URL, "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := gateway.TierKyc(context.Background(), 42)
	if err != nil || !*result.Verified {
		t.Fatalf("approval not shared: %v", err)
	}
	if result.VerificationURL != server.URL+*result.VerificationPath {
		t.Fatal("incorrect verification destination")
	}
}
func TestTierKycRejectsMissingFieldsWrongIdentityExpiredAndSandboxApproval(t *testing.T) {
	cases := map[string]func(*TierKycStatus){
		"wrong user":        func(s *TierKycStatus) { s.ExternalUserID = "43" },
		"missing verified":  func(s *TierKycStatus) { s.Verified = nil },
		"missing link":      func(s *TierKycStatus) { s.Linked = nil },
		"missing policy":    func(s *TierKycStatus) { s.WithdrawalRequired = nil },
		"stale response":    func(s *TierKycStatus) { s.CheckedAt = time.Now().Add(-time.Hour) },
		"expired":           func(s *TierKycStatus) { expired := time.Now().Add(-time.Minute); s.ValidUntil = &expired },
		"sandbox":           func(s *TierKycStatus) { s.Environment = "sandbox" },
		"wrong status":      func(s *TierKycStatus) { s.Status = "rejected" },
		"redirect":          func(s *TierKycStatus) { path := "https://attacker.test"; s.VerificationPath = &path },
		"unlinked approval": func(s *TierKycStatus) { s.Linked = kycBool(false) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			state := validTierKyc()
			mutate(&state)
			if err := state.validate(42, time.Now()); err == nil {
				t.Fatal("unsafe approval accepted")
			}
		})
	}
}
func TestTierKycUUIDIdentityUsesExistingResolver(t *testing.T) {
	uuid := "11111111-1111-4111-8111-111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["external_user_id"] != uuid || r.Header.Get("X-Signet-Identity-Version") != "uuid-v1" {
			t.Error("UUID protocol not reused")
		}
		state := validTierKyc()
		state.ExternalUserID = uuid
		json.NewEncoder(w).Encode(state)
	}))
	defer server.Close()
	g, _ := New(server.URL, "test")
	g.identityResolver = func(context.Context, uint64) (string, error) { return uuid, nil }
	g.identityReverse = func(context.Context, string) (uint64, error) { return 42, nil }
	result, err := g.TierKyc(context.Background(), 42)
	if err != nil || result.ExternalUserID != fmt.Sprint(42) {
		t.Fatalf("UUID response not bound to local owner: %v", err)
	}
}

package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWalletLedgerBalancesAreValidated(t *testing.T) {
	for _, tc := range []struct {
		name     string
		balances map[string]AssetBalance
		allowed  bool
	}{
		{"valid", map[string]AssetBalance{"USDT": {"20", "0", "0"}, "SIGNET": {"1.5", "1000", "2"}}, true},
		{"negative", map[string]AssetBalance{"USDT": {"20", "0", "0"}, "SIGNET": {"-1", "1000", "2"}}, false},
		{"missing", map[string]AssetBalance{"USDT": {"20", "0", "0"}}, false},
		{"disagreement", map[string]AssetBalance{"USDT": {"21", "0", "0"}, "SIGNET": {"1", "0", "0"}}, false},
		{"malformed", map[string]AssetBalance{"USDT": {"20", "0", "0"}, "SIGNET": {"1", "", "0"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(Wallet{Address: "0x1234567890123456789012345678901234567890", ChainID: 97, Asset: "USDT", Available: "20", ProvisioningStatus: "ready", Balances: tc.balances})
			}))
			defer server.Close()
			g, _ := New(server.URL, "test")
			wallet, err := g.EnsureWallet(context.Background(), 42)
			if (err == nil) != tc.allowed {
				t.Fatalf("validation returned %v", err)
			}
			if tc.allowed && wallet.Balances["SIGNET"].Locked != "1000" {
				t.Fatal("locked balance lost")
			}
		})
	}
}

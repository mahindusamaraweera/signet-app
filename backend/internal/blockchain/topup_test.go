package blockchain

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTopupManifestEchoAndHashAreVerified(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Rows     []map[string]string `json:"commissions"`
				Buyer    string              `json:"external_user_id"`
				Purchase string              `json:"external_purchase_id"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Error("invalid request")
			}
			if r.Header.Get("Idempotency-Key") != "signet-app:topup-manifest:7" {
				t.Error("unstable manifest key")
			}
			canonical, _ := json.Marshal(body.Rows)
			hash := fmt.Sprintf("%x", sha256.Sum256(canonical))
			if invalid {
				hash = fmt.Sprintf("%064d", 0)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"sealed": true, "manifest_hash": hash, "external_user_id": body.Buyer, "external_purchase_id": body.Purchase})
		}))
		gateway, _ := New(server.URL, "test")
		err := gateway.SealTopupManifest(context.Background(), PurchaseIntent{ID: 7, UserID: 42}, []TopupCommission{{ID: 9, Recipient: 43, Kind: "direct_team", USD: "50"}})
		server.Close()
		if (err != nil) != invalid {
			t.Fatalf("invalid=%v: %v", invalid, err)
		}
	}
}

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

func TestCompanyManifestBindsFullSourceEvidenceAndCustodyResponse(t *testing.T) {
	for _, scenario := range []string{"sealed", "settled", "legacy", "wrong hash", "wrong buyer", "missing state"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Buyer    string              `json:"external_user_id"`
					Purchase string              `json:"external_purchase_id"`
					Rows     []map[string]string `json:"commissions"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.URL.Path != "/api/v1/integration/package-purchases/company-manifest" || body.Buyer != "42" || body.Purchase != "1" || len(body.Rows) != 1 || body.Rows[0]["commission_usd"] != "75" || body.Rows[0]["external_earn_log_id"] != "9" {
					t.Error("manifest lost source evidence")
				}
				raw, _ := json.Marshal(body.Rows)
				hash := fmt.Sprintf("%x", sha256.Sum256(raw))
				enabled, sealed, settled := true, true, scenario == "settled"
				buyer := "42"
				if scenario == "legacy" {
					enabled = false
					sealed = false
					hash = ""
				}
				if scenario == "wrong hash" {
					hash = "bad"
				}
				if scenario == "wrong buyer" {
					buyer = "43"
				}
				result := CompanySettlementResult{Enabled: &enabled, Sealed: &sealed, Settled: &settled, Hash: hash, Buyer: buyer, Purchase: "1"}
				if scenario == "missing state" {
					result.Enabled = nil
				}
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			gateway, _ := New(server.URL, "test")
			_, err := gateway.SealFirstCompanyManifest(context.Background(), PurchaseIntent{ID: 1, UserID: 42, Kind: "first", Price: "100"}, []CommissionInstruction{{ID: 7, EarnLogID: 9, PurchaseID: 1, BuyerID: 42, RecipientID: 77, Kind: "direct_team", USD: "75"}})
			invalid := scenario == "wrong hash" || scenario == "wrong buyer" || scenario == "missing state"
			if (err != nil) != invalid {
				t.Fatalf("unexpected validation: %v", err)
			}
		})
	}
}

func TestCompanyManifestRejectsOverspendingDuplicatesAndForeignEvidenceBeforeRequest(t *testing.T) {
	gateway, _ := New("http://127.0.0.1:1", "test")
	intent := PurchaseIntent{ID: 1, UserID: 42, Kind: "first", Price: "100"}
	row := CommissionInstruction{ID: 7, EarnLogID: 9, PurchaseID: 1, BuyerID: 42, RecipientID: 77, Kind: "direct_team", USD: "75"}
	oversized := row
	oversized.USD = "100.000000000000000001"
	foreign := row
	foreign.PurchaseID = 2
	for _, rows := range [][]CommissionInstruction{{oversized}, {row, row}, {foreign}} {
		if _, err := gateway.SealFirstCompanyManifest(context.Background(), intent, rows); err != ErrUnavailable {
			t.Fatalf("unsafe manifest accepted: %v", err)
		}
	}
}

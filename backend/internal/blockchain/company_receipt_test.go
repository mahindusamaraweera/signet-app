package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoveredCompanyReceiptIsReadOnlyAndRejectsUnprovenCompletion(t *testing.T) {
	for _, scenario := range []string{"confirmed", "pending", "wrong order", "missing receipt", "zero receipt", "wrong buyer"} {
		t.Run(scenario, func(t *testing.T) {
			uuid := "01a139c5-c200-7000-8000-0000000001cc"
			purchase, _ := OperationUUID(uuid, "purchase", "1")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/integration/package-purchases/company-status" || r.Method != "POST" {
					t.Error("payment endpoint called")
				}
				var body map[string]interface{}
				json.NewDecoder(r.Body).Decode(&body)
				if body["external_user_id"] != uuid || body["external_purchase_id"] != purchase {
					t.Error("unscoped identity")
				}
				result := map[string]interface{}{"external_user_id": uuid, "external_purchase_id": purchase, "native_order_id": 5, "package_price_usdt": "1000", "sealed": true, "settled": true, "manifest_hash": strings.Repeat("a", 64), "settlement_tx_hash": "0x" + strings.Repeat("b", 64), "settlement_usdt": "270.478", "settlement_confirmed_at": "2026-10-15 17:03:25"}
				switch scenario {
				case "pending":
					result["settled"] = false
					result["settlement_usdt"] = nil
					result["settlement_confirmed_at"] = nil
				case "wrong order":
					result["native_order_id"] = 6
				case "missing receipt":
					result["settlement_tx_hash"] = nil
				case "zero receipt":
					result["settlement_usdt"] = "0"
				case "wrong buyer":
					result["external_user_id"] = productionCustomer
				}
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			g, _ := New(server.URL, "test", "production")
			g.UseUUIDIdentity(func(context.Context, uint64) (string, error) { return uuid, nil }, nil)
			_, err := g.Recovered460CompanyReceipt(context.Background())
			valid := scenario == "confirmed" || scenario == "pending"
			if (err == nil) != valid {
				t.Fatalf("scenario %s err=%v", scenario, err)
			}
		})
	}
}

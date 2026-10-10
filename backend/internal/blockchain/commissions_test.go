package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommissionPreparationIdentityBinding(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if r.URL.Path != "/api/v1/integration/commissions/prepare" || r.Header.Get("Idempotency-Key") != "signet-app:commission-prepare:7" || body["external_user_id"] != "42" || body["external_recipient_id"] != "77" || body["commission_usd"] != "50" {
				t.Error("incorrect trusted source identity")
			}
			recipient := "77"
			if mismatch {
				recipient = "78"
			}
			json.NewEncoder(w).Encode(CommissionPreparation{ID: "1", OutboxID: "7", EarnLogID: "18", RecipientID: recipient, Kind: "direct_team", USD: "50.000000000000000000", RecipientAddress: "0x1234567890123456789012345678901234567890", Status: "awaiting_authorization", Quote: json.RawMessage(`{"signet_price_usdt":"0.125","signet_amount":"400"}`)})
		}))
		g, _ := New(server.URL, "test")
		_, err := g.PrepareCommission(context.Background(), CommissionInstruction{ID: 7, EarnLogID: 18, PurchaseID: 1, BuyerID: 42, RecipientID: 77, Kind: "direct_team", USD: "50"})
		server.Close()
		if (err != nil) != mismatch {
			t.Fatalf("identity validation mismatch=%v error=%v", mismatch, err)
		}
	}
}

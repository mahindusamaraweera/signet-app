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

const productionCustomer = "01a13b42-28d0-7000-8000-0000000001cd"
const productionUpline = "11111111-1111-4111-8111-111111111111"

func uuidGateway(t *testing.T, server string) *Gateway {
	t.Helper()
	g, err := New(server, "test", "production")
	if err != nil {
		t.Fatal(err)
	}
	g.UseUUIDIdentity(func(_ context.Context, id uint64) (string, error) {
		if id == 461 {
			return productionCustomer, nil
		}
		if id == 77 {
			return productionUpline, nil
		}
		return "", fmt.Errorf("unknown UUID")
	}, func(_ context.Context, id string) (uint64, error) {
		if id == productionCustomer {
			return 461, nil
		}
		return 0, ErrRejected
	})
	return g
}
func TestUUIDWireIsCustomerScopedStableAndNeverSendsNumericCustomerIDs(t *testing.T) {
	seen := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if r.Header.Get("X-Signet-Identity-Version") != "uuid-v1" || body["external_user_id"] != productionCustomer {
			t.Error("numeric customer identity escaped")
		}
		if body["package_id"] != "3" || body["package_price_usdt"] != "1000" {
			t.Error("local catalog/financial terms changed")
		}
		op, _ := OperationUUID(productionCustomer, "purchase", "2")
		if body["external_purchase_id"] != op {
			t.Error("purchase not scoped by customer UUID")
		}
		if seen != "" && seen != op {
			t.Error("retry changed operation identity")
		}
		seen = op
		if body["identity_bindings"].(map[string]interface{})[productionCustomer] != "461" {
			t.Error("missing explicit legacy reference")
		}
		fmt.Fprint(w, `{"status":"awaiting_deposit","available_usdt":"1130.99","remaining_usdt":"0","deposit_seen":true,"required_usdt":"1130","service_fee_usdt":"130"}`)
	}))
	defer server.Close()
	g := uuidGateway(t, server.URL)
	for i := 0; i < 2; i++ {
		if _, err := g.SyncPurchase(context.Background(), PurchaseIntent{ID: 2, UserID: 461, PackageID: 3, Price: "1000", Required: "1130", Fee: "130", BonusPercent: 400, Kind: "first"}); err != nil {
			t.Fatal(err)
		}
	}
	other, _ := OperationUUID(productionUpline, "purchase", "2")
	if other == seen {
		t.Fatal("same numeric purchase collided for different UUID owners")
	}
}
func TestUUIDCommissionAndManifestUseIdenticalCanonicalWireRows(t *testing.T) {
	var prepared map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path == "/api/v1/integration/commissions/prepare" {
			prepared = body
			if body["external_recipient_id"] != productionUpline {
				t.Error("recipient not a UUID")
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"id": "10", "external_outbox_id": body["external_outbox_id"], "external_earn_log_id": body["external_earn_log_id"], "external_recipient_id": body["external_recipient_id"], "commission_kind": "direct_team", "commission_usd": "400", "status": "awaiting_authorization", "recipient_address": "0x1111111111111111111111111111111111111111", "quote": map[string]string{"signet_price_usdt": "0.1", "signet_amount": "4000"}})
			return
		}
		rows := body["commissions"].([]interface{})
		row := rows[0].(map[string]interface{})
		for _, key := range []string{"external_outbox_id", "external_earn_log_id", "external_recipient_id"} {
			if row[key] != prepared[key] {
				t.Error("manifest differs from prepared UUID commission")
			}
		}
		raw, _ := json.Marshal(rows)
		json.NewEncoder(w).Encode(map[string]interface{}{"external_user_id": body["external_user_id"], "external_purchase_id": body["external_purchase_id"], "enabled": true, "sealed": true, "settled": false, "manifest_hash": fmt.Sprintf("%x", sha256.Sum256(raw))})
	}))
	defer server.Close()
	g := uuidGateway(t, server.URL)
	c := CommissionInstruction{ID: 7, EarnLogID: 9, PurchaseID: 2, BuyerID: 461, RecipientID: 77, Kind: "direct_team", USD: "400"}
	if _, err := g.PrepareCommission(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if _, err := g.SealFirstCompanyManifest(context.Background(), PurchaseIntent{ID: 2, UserID: 461, Price: "1000", Kind: "first"}, []CommissionInstruction{c}); err != nil {
		t.Fatal(err)
	}
}
func TestUUIDMissingNeverFallsBackToNumericIdentity(t *testing.T) {
	g := uuidGateway(t, "http://127.0.0.1:1")
	if _, err := g.EnsureWallet(context.Background(), 999); err == nil {
		t.Fatal("missing UUID fell back to numeric ID")
	}
}
func TestUUIDTreasuryMarkerIsNotAUserIdentity(t *testing.T) {
	g := uuidGateway(t, "http://127.0.0.1:1")
	wire, _, err := g.prepareUUIDWire(context.Background(), map[string]interface{}{"external_user_id": "461", "external_recipient_id": "1"})
	if err != nil || wire["external_recipient_id"] != "treasury" {
		t.Fatal(err)
	}
	if _, _, err := g.prepareUUIDWire(context.Background(), wire); err != nil {
		t.Fatal("already-normalized treasury marker failed", err)
	}
}

func TestOperationUUIDMatchesIndependentPHPContractVector(t *testing.T) {
	actual, err := OperationUUID(productionCustomer, "purchase", "2")
	if err != nil || actual != "2fcc99ee-056a-567e-a306-cba447012f34" {
		t.Fatal(actual, err)
	}
}

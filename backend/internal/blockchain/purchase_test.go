package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPurchaseCommercialSnapshotTransport(t *testing.T) {
	for _, kind := range []string{"first", "topup"} {
		t.Run(kind, func(t *testing.T) {
			bonus := int64(400)
			if kind == "topup" {
				bonus = 0
			}
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body struct {
					Fee      string `json:"service_fee_usdt"`
					Required string `json:"required_usdt"`
					Bonus    int64  `json:"bonus_percent"`
					Kind     string `json:"purchase_kind"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Fee != "130" || body.Required != "1130" || body.Bonus != bonus || body.Kind != kind {
					t.Errorf("incorrect commercial snapshot: %+v", body)
				}
				w.Write([]byte(`{"status":"awaiting_deposit","available_usdt":"1000","deposit_seen":true,"required_usdt":"1130.000000000000000000","service_fee_usdt":"130.000000000000000000"}`))
			}))
			defer s.Close()
			g, _ := New(s.URL, "test")
			intent := PurchaseIntent{ID: 7, UserID: 42, Price: "1000", Fee: "130", Required: "1130", BonusPercent: bonus, Kind: kind}
			result, err := g.SyncPurchase(context.Background(), intent)
			if err != nil || result.Remaining != "130.000000000000000000" {
				t.Fatalf("snapshot synchronization failed: %+v %v", result, err)
			}
			intent.Required = "1000"
			if _, err := g.SyncPurchase(context.Background(), intent); err == nil || calls != 1 {
				t.Fatal("invalid snapshot reached custody")
			}
		})
	}
}

func TestFundingRemaining(t *testing.T) {
	for _, tc := range []struct{ price, available, want string }{
		{"100", "60", "40.000000000000000000"},
		{"100", "100", "0.000000000000000000"},
		{"100", "120", "0.000000000000000000"},
		{"1", "0.999999999999999999", "0.000000000000000001"},
	} {
		got, err := FundingRemaining(tc.price, tc.available)
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %s %v", tc, got, err)
		}
	}
	for _, a := range []string{"-1", "1e5", "NaN", "", "0.0000000000000000001"} {
		if _, err := FundingRemaining("100", a); err == nil {
			t.Fatalf("accepted %q", a)
		}
	}
}

func TestPurchaseSyncProtectsPartialAndRejectsUnconfirmedCompletion(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"status":"awaiting_deposit","available_usdt":"60","deposit_seen":false}`, true},
		{`{"status":"awaiting_deposit","available_usdt":"100","deposit_seen":true,"required_usdt":"120","service_fee_usdt":"20"}`, true},
		{`{"status":"awaiting_deposit","available_usdt":"100","deposit_seen":true,"required_usdt":"121","service_fee_usdt":"20"}`, false},
		{`{"status":"completed","available_usdt":"20","deposit_seen":true,"order_id":"1","swap_confirmed":true,"vesting_allocated":true}`, true},
		{`{"status":"completed","available_usdt":"20","deposit_seen":true,"order_id":"1","swap_confirmed":false,"vesting_allocated":true}`, false},
		{`{"status":"awaiting_deposit","available_usdt":"0"}`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Idempotency-Key") != "signet-app:purchase:7" {
					t.Error("unstable purchase key")
				}
				w.Write([]byte(tc.body))
			}))
			defer s.Close()
			g, _ := New(s.URL, "test")
			result, err := g.SyncPurchase(context.Background(), PurchaseIntent{ID: 7, UserID: 42, Price: "100"})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.valid && !*result.DepositSeen {
				t.Error("partial deposit must protect placement")
			}
			if tc.valid && result.Required == "120" && result.Remaining != "20.000000000000000000" {
				t.Fatal("service fee must be included in remaining funding")
			}
		})
	}
}

package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"signet-backend/internal/withdrawalsecurity"
	"strings"
	"testing"
)

func TestWithdrawalCancellationRequiresExactExplicitResponse(t *testing.T) {
	a := withdrawalsecurity.WithdrawalAssertion{AuthorizationID: strings.Repeat("a", 64), ExternalUserID: "42", SecurityVersion: "2", AddressID: strings.Repeat("b", 64), Destination: "0x" + strings.Repeat("c", 40), Network: "BSC", Asset: "SIGNET", Gross: "100.000000000000000000", Fee: "3.000000000000000000", Net: "97.000000000000000000", IssuedAt: "1800000000", ExpiresAt: "1800000060"}
	for _, scenario := range []string{"tombstone", "refund", "wrong identity", "wrong status", "missing id", "unsafe"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "unsafe" {
					w.WriteHeader(409)
					return
				}
				if r.URL.Path != "/api/v1/integration/withdrawals/cancel" || r.Header.Get("Idempotency-Key") != "signet-app:withdrawal-cancel:"+a.AuthorizationID {
					t.Error("wrong cancellation transport")
				}
				body := map[string]interface{}{"authorization_id": a.AuthorizationID, "status": "cancelled", "withdrawal_id": nil}
				switch scenario {
				case "refund":
					body["withdrawal_id"] = "7"
				case "wrong identity":
					body["authorization_id"] = strings.Repeat("d", 64)
				case "wrong status":
					body["status"] = "pending"
				case "missing id":
					delete(body, "withdrawal_id")
				}
				json.NewEncoder(w).Encode(body)
			}))
			defer server.Close()
			g, _ := New(server.URL, "credential")
			result, err := g.CancelWithdrawal(context.Background(), a, strings.Repeat("e", 64))
			valid := scenario == "tombstone" || scenario == "refund"
			if (err == nil) != valid || (!valid && result != nil) {
				t.Fatalf("wrong cancellation confirmation: %v", err)
			}
			if scenario == "unsafe" && !errors.Is(err, ErrCancellationReviewRequired) {
				t.Fatal("unsafe cancellation treated as normal retry")
			}
		})
	}
}

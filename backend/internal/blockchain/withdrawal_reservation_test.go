package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"signet-backend/internal/withdrawalsecurity"
)

func TestWithdrawalReservationMatchesSignedTermsAndStableIdentity(t *testing.T) {
	a := withdrawalsecurity.WithdrawalAssertion{AuthorizationID: strings.Repeat("a", 64), ExternalUserID: "42", SecurityVersion: "2", AddressID: strings.Repeat("b", 64), Destination: "0x" + strings.Repeat("c", 40), Network: "BSC", Asset: "SIGNET", Gross: "100.000000000000000000", Fee: "3.000000000000000000", Net: "97.000000000000000000", IssuedAt: "1800000000", ExpiresAt: "1800000060"}
	for _, scenario := range []string{"valid", "owner", "amount", "fee", "net", "destination", "status", "id"} {
		t.Run(scenario, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/api/v1/integration/withdrawals/reserve" || r.Header.Get("Idempotency-Key") != "signet-app:withdrawal:"+a.AuthorizationID || r.Header.Get("Authorization") != "Bearer credential" {
					t.Error("wrong reservation transport")
				}
				var body struct {
					Source    string                                 `json:"source"`
					User      string                                 `json:"external_user_id"`
					Assertion withdrawalsecurity.WithdrawalAssertion `json:"assertion"`
					Signature string                                 `json:"signature"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Source != "signet-app" || body.User != "42" || body.Assertion != a || body.Signature != strings.Repeat("d", 64) {
					t.Error("changed signed approval")
				}
				result := WithdrawalReservation{ID: "7", ExternalUserID: "42", Status: "external_security_pending", Asset: "SIGNET", Amount: a.Gross, Fee: a.Fee, NetAmount: a.Net, Destination: a.Destination}
				switch scenario {
				case "owner":
					result.ExternalUserID = "43"
				case "amount":
					result.Amount = "101"
				case "fee":
					result.Fee = "4"
				case "net":
					result.NetAmount = "96"
				case "destination":
					result.Destination = "0x" + strings.Repeat("e", 40)
				case "status":
					result.Status = "completed"
				case "id":
					result.ID = "0"
				}
				json.NewEncoder(w).Encode(result)
			}))
			defer server.Close()
			g, err := New(server.URL, "credential")
			if err != nil {
				t.Fatal(err)
			}
			result, err := g.ReserveWithdrawal(context.Background(), a, strings.Repeat("d", 64))
			if (err == nil) != (scenario == "valid") || calls != 1 {
				t.Fatalf("wrong reservation boundary: %v", err)
			}
			if err != nil && result != nil {
				t.Fatal("untrusted response returned")
			}
		})
	}
}

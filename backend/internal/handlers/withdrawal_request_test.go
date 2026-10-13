package handlers

import (
	"net/http/httptest"
	"signet-backend/internal/app"
	"strings"
	"testing"
)

func TestWithdrawalUserActionsRejectAnonymousRequestsWithoutProofDisclosure(t *testing.T) {
	for _, action := range []string{"prepare", "email", "confirm", "queue", "cancel"} {
		t.Run(action, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1/blockchain/withdrawals/"+action, strings.NewReader(`{"password":"never disclose","proof":"never disclose"}`))
			w := httptest.NewRecorder()
			if action == "cancel" {
				withdrawalCancelHandler(&app.Deps{})(w, r)
			} else {
				withdrawalRequestHandler(&app.Deps{}, action)(w, r)
			}
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "never disclose") {
				t.Fatal("unauthenticated withdrawal request did not fail closed")
			}
		})
	}
}

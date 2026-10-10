package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"signet-backend/internal/app"
)

func TestWithdrawalEmailSecurityFailsClosedWithoutService(t *testing.T) {
	for _, handler := range []http.HandlerFunc{withdrawalEmailIssueHandler(&app.Deps{}), withdrawalEmailVerifyHandler(&app.Deps{}), withdrawalAuthenticatorHandler(&app.Deps{}, false), withdrawalAuthenticatorHandler(&app.Deps{}, true)} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", "/security/email", nil))
		if response.Code != 503 || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("unavailable security service did not fail closed")
		}
	}
}

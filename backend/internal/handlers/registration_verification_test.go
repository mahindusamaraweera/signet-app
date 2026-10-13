package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"signet-backend/internal/app"
)

func TestRegistrationFailsClosedWithoutVerificationService(t *testing.T) {
	deps := &app.Deps{} // No DB: rejection must happen before account writes.
	for _, handler := range []http.HandlerFunc{registerStep1Handler(deps), verifyRegistrationHandler(deps)} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", "/register", nil))
		if response.Code != 503 {
			t.Fatalf("got %d, expected unavailable", response.Code)
		}
		if len(response.Result().Cookies()) != 0 {
			t.Fatal("unverified registration received a session")
		}
	}
}

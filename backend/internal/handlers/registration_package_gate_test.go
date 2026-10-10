package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"signet-backend/internal/app"
	"signet-backend/internal/config"
)

// Before email verification there is no authenticated source user. Direct
// package URLs and forged package submits must fail before touching storage.
func TestBlockchainPackageSelectionRequiresVerifiedRegistrationSession(t *testing.T) {
	d := &app.Deps{Cfg: &config.Config{BlockchainEnabled: true}}
	t.Run("direct URL", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/register/step2/42", nil)
		route := chi.NewRouteContext()
		route.URLParams.Add("id", "42")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
		response := httptest.NewRecorder()
		registerStep2FormHandler(d)(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("got %d", response.Code)
		}
	})
	t.Run("forged submission", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/register/step2", strings.NewReader(`{"package":"1","newUserID":"42"}`))
		response := httptest.NewRecorder()
		registerStep2SubmitHandler(d)(response, req)
		if response.Code != http.StatusForbidden {
			t.Fatalf("got %d", response.Code)
		}
	})
}

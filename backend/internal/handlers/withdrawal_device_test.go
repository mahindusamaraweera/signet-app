package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
)

func TestDeferredDeviceVerificationHasNoPublicRoute(t *testing.T) {
	router := chi.NewRouter()
	RegisterBlockchainRoutes(router, &app.Deps{})
	err := chi.Walk(router, func(method, route string, handler http.Handler, middleware ...func(http.Handler) http.Handler) error {
		if strings.Contains(route, "/security/device") {
			t.Fatalf("deferred device endpoint remains mounted: %s %s", method, route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithdrawalDeviceCookieIsPrivateAndAccountScoped(t *testing.T) {
	credential := strings.Repeat("a", 64)
	first := withdrawalDeviceCookie(42, credential, true)
	second := withdrawalDeviceCookie(43, credential, true)
	if first.Name == second.Name || !first.HttpOnly || !first.Secure || first.SameSite != http.SameSiteStrictMode || first.Path != "/" || first.Domain != "" {
		t.Fatal("unsafe device cookie")
	}
	if first.Value != credential || first.MaxAge != 90*24*60*60 {
		t.Fatal("credential changed")
	}
}

func TestUnavailableDeviceServiceIssuesNoCredential(t *testing.T) {
	response := httptest.NewRecorder()
	withdrawalDeviceRecordHandler(&app.Deps{}).ServeHTTP(response, httptest.NewRequest("POST", "/security/device", nil))
	if response.Code != 503 || len(response.Result().Cookies()) != 0 || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unavailable device service did not fail closed")
	}
}

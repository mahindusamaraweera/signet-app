package handlers

import (
	"net/http/httptest"
	"signet-backend/internal/app"
	"strings"
	"testing"
)

func TestAddressEndpointsRejectAnonymousRequestsWithoutSideEffects(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		for _, revoke := range []bool{false, true} {
			r := httptest.NewRequest("POST", "/security/address", strings.NewReader(`{"user_id":42,"password":"password"}`))
			w := httptest.NewRecorder()
			withdrawalAddressActionHandler(&app.Deps{}, confirm, revoke).ServeHTTP(w, r)
			if w.Code != 401 || w.Header().Get("Cache-Control") != "no-store" || len(w.Result().Cookies()) != 0 {
				t.Fatal("anonymous address request did not fail closed")
			}
		}
	}
}

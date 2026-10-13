package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"signet-backend/internal/app"
	"signet-backend/internal/config"
)

func TestAutomatedRegistrationCannotUseLegacyAccountDeletion(t *testing.T) {
	for _, cfg := range []*config.Config{
		{BlockchainEnabled: true},
		{RegistrationExpiryEnabled: true},
		{BlockchainEnabled: true, RegistrationExpiryEnabled: true},
	} {
		// Deliberately no database: protection must reject before any SQL,
		// regardless of placement-log URL contents or account balances.
		deps := &app.Deps{Cfg: cfg}
		response := httptest.NewRecorder()
		userParentLogsDestroyHandler(deps)(response, httptest.NewRequest(http.MethodDelete, "/user-parent-logs/1", nil))
		if response.Code != http.StatusConflict {
			t.Fatalf("account deletion was not blocked: %d %s", response.Code, response.Body.String())
		}
	}
}

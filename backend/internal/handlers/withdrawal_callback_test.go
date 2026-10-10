package handlers

import (
	"net/http/httptest"
	"signet-backend/internal/app"
	"signet-backend/internal/config"
	"strings"
	"testing"
)

func TestWithdrawalCallbackRequiresSeparateServiceCredential(t *testing.T) {
	for _, scenario := range []string{"disabled", "browser session", "wrong token", "key reuse", "authenticated unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := &config.Config{BlockchainEnabled: true, BlockchainWithdrawalSubmissionEnabled: true, SessionSecret: strings.Repeat("s", 32), BlockchainServiceToken: strings.Repeat("b", 32), BlockchainWithdrawalAuthorizationKey: strings.Repeat("k", 32), BlockchainWithdrawalCallbackToken: strings.Repeat("c", 32)}
			status := 401
			authorization := "Bearer " + strings.Repeat("x", 32)
			if scenario == "disabled" {
				cfg.BlockchainWithdrawalSubmissionEnabled = false
				status = 404
			}
			if scenario == "browser session" {
				authorization = "Bearer " + cfg.SessionSecret
			}
			if scenario == "key reuse" {
				cfg.BlockchainWithdrawalCallbackToken = cfg.BlockchainWithdrawalAuthorizationKey
				status = 503
			}
			if scenario == "authenticated unavailable" {
				authorization = "Bearer " + cfg.BlockchainWithdrawalCallbackToken
				status = 503
			}
			r := httptest.NewRequest("POST", "/api/v1/integration/withdrawals/revalidate", strings.NewReader(`{}`))
			r.Header.Set("Authorization", authorization)
			w := httptest.NewRecorder()
			withdrawalCallbackHandler(&app.Deps{Cfg: cfg})(w, r)
			if w.Code != status || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("wrong callback boundary: %d", w.Code)
			}
		})
	}
}

package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"signet-backend/internal/withdrawalsecurity"
)

func TestPasswordResetRequestDoesNotDiscloseIssuanceOutcome(t *testing.T) {
	var baseline string
	for _, outcome := range []error{nil, withdrawalsecurity.ErrChallengeCooldown, errors.New("SMTP secret"), errors.New("database error")} {
		calls := 0
		handler := passwordEmailRequestHandler(func(_ *http.Request, email string) error {
			calls++
			if email != "user@example.com" {
				t.Fatal("wrong recipient")
			}
			return outcome
		})
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("POST", "/api/v1/password/email", strings.NewReader(`{"email":"user@example.com"}`)))
		if w.Code != http.StatusOK || calls != 1 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("nonuniform response")
		}
		if baseline == "" {
			baseline = w.Body.String()
		}
		if baseline != w.Body.String() {
			t.Fatal("issuance outcome disclosed")
		}
	}
}

func TestPasswordResetRequestRejectsMalformedEmailWithoutIssuing(t *testing.T) {
	handler := passwordEmailRequestHandler(func(_ *http.Request, _ string) error { t.Fatal("invalid request issued"); return nil })
	for _, body := range []string{`{"email":"invalid"}`, `{`} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("POST", "/api/v1/password/email", strings.NewReader(body)))
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("wrong status %d", w.Code)
		}
	}
}

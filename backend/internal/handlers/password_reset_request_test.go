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

func TestPasswordResetRequestReportsUnknownEmail(t *testing.T) {
	handler := passwordEmailRequestHandler(func(_ *http.Request, _ string) error { return errPasswordResetUnknownEmail })
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest("POST", "/api/v1/password/email", strings.NewReader(`{"email":"abcd@gmail.com"}`)))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "couldn't find an account") {
		t.Fatalf("unknown email not reported: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "success") {
		t.Fatal("unknown email answered as success")
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

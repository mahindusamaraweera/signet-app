package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRetiredKycCannotUploadApproveOrDelete(t *testing.T) {
	for _, method := range []string{"POST", "PUT", "DELETE", "GET"} {
		rec := httptest.NewRecorder()
		retiredKycHandler(rec, httptest.NewRequest(method, "/api/v1/kyc/1/verify", strings.NewReader(`{"is_verified":true}`)))
		if rec.Code != http.StatusGone {
			t.Fatalf("%s: got %d", method, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "kyc_method_retired") {
			t.Fatal("missing retirement response")
		}
	}
}

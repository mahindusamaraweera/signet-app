package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSecurityLimiterIgnoresForgedForwardedAddresses(t *testing.T) {
	handler := SecurityRateLimit(2, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < 3; i++ {
		request := httptest.NewRequest("POST", "/security", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("X-Forwarded-For", "198.51.100."+string(rune('1'+i)))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		want := 204
		if i == 2 {
			want = 429
		}
		if response.Code != want {
			t.Fatalf("request %d: got %d", i, response.Code)
		}
	}
	request := httptest.NewRequest("POST", "/security", nil)
	request.RemoteAddr = "192.0.2.2:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal("different peer blocked")
	}
}

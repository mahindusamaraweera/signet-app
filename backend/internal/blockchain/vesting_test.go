package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVestingSwitchUsesAuthenticatedIdentityAndStableRetry(t *testing.T) {
	requestID := "12345678-1234-4234-8234-123456789012"
	commits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["external_user_id"] != "42" || body["source"] != "signet-app" || body["schedule_id"] != float64(7) {
			t.Error("incorrect scoped identity")
		}
		switch r.URL.Path {
		case "/api/v1/integration/vesting/switch/preview":
			w.Write([]byte(`{"preview":{"confirmation_token":"opaque"}}`))
		case "/api/v1/integration/vesting/switch/commit":
			commits++
			if r.Header.Get("Idempotency-Key") != "signet-app:vesting-switch:"+requestID || body["request_id"] != requestID || body["confirmation_token"] != "opaque" || body["consent"] != true {
				t.Error("retry terms changed")
			}
			w.Write([]byte(`{"schedule":{"id":7}}`))
		default:
			t.Error("unexpected endpoint")
		}
	}))
	defer server.Close()
	g, _ := New(server.URL, "test")
	if _, err := g.PreviewVestingSwitch(context.Background(), 42, 7, "existing"); err != nil {
		t.Fatal(err)
	}
	body := VestingSwitch{ScheduleID: 7, RequestID: requestID, ConfirmationToken: "opaque", Consent: true}
	for i := 0; i < 2; i++ {
		if _, err := g.CommitVestingSwitch(context.Background(), 42, body); err != nil {
			t.Fatal(err)
		}
	}
	if commits != 2 {
		t.Fatal("missing retry")
	}
}

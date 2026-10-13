package handlers

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"signet-backend/internal/app"
	"signet-backend/internal/auth"
)

func TestFilterLinesAppliesSinceGrepAndLimit(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.Local)
	lines := []string{
		"2026/10/11 09:00:00 purchase intent 1: old",          // before since
		"2026/10/12 10:00:00 purchase intent 7: unavailable",  // match
		"2026/10/12 10:30:00 something else",                  // no grep match
		"2026/10/12 11:00:00 PURCHASE INTENT 8: http 401",     // case-insensitive match
		"no timestamp purchase intent continuation",           // kept (no prefix)
		"2026/10/12 11:30:00 purchase intent 9: invalid json", // match
	}
	got, truncated := filterLines(lines, now.Add(-6*time.Hour), "purchase intent", 2)
	want := []string{"no timestamp purchase intent continuation", "2026/10/12 11:30:00 purchase intent 9: invalid json"}
	if !reflect.DeepEqual(got, want) || !truncated {
		t.Fatalf("got %q truncated=%v", got, truncated)
	}
	all, truncated := filterLines(lines, now.Add(-6*time.Hour), "", 100)
	if len(all) != 5 || truncated {
		t.Fatalf("since filter: got %d lines (truncated=%v)", len(all), truncated)
	}
}

func TestAdminLogsRequiresLogin(t *testing.T) {
	r := chi.NewRouter()
	RegisterLogRoutes(r, &app.Deps{Auth: &auth.Service{Secret: []byte("0123456789abcdef0123456789abcdef")}})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs", nil))
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous request got %d", rec.Code)
	}
}

func TestAdminLogsRejectsBadParameters(t *testing.T) {
	h := adminLogsHandler(&app.Deps{})
	for _, q := range []string{"since=999h", "since=-1h", "since=abc", "limit=0", "limit=99999", "grep=a%0Ab"} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?"+q, nil))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: got %d, want 422", q, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?since=1h&grep=x&limit=10&source=memory", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid request got %d: %s", rec.Code, rec.Body.String())
	}
}

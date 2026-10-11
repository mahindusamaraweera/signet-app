package auth

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"signet-backend/internal/models"
)

func TestSessionCredentialRevisionInvalidatesOldAndLegacySessions(t *testing.T) {
	s := &Service{Secret: []byte(strings.Repeat("k", 32))}
	u := &models.User{ID: 42, Password: "bcrypt-hash"}
	c := &sessionClaims{UserID: 42, CredentialVersion: s.credentialVersion(u)}
	if !s.sessionMatches(c, u) || c.CredentialVersion == u.Password {
		t.Fatal("invalid credential binding")
	}
	u.Password = "new-bcrypt-hash"
	if s.sessionMatches(c, u) {
		t.Fatal("password change kept old session")
	}
	c.CredentialVersion = ""
	if s.sessionMatches(c, u) {
		t.Fatal("legacy unversioned session accepted")
	}
	c.CredentialVersion = s.credentialVersion(u)
	c.UserID = 43
	if s.sessionMatches(c, u) {
		t.Fatal("other identity accepted")
	}
}

// Registration verification used to issue the session from a bare
// models.User{ID, Role}: its credential version was derived from an empty
// password, so the very next request rejected it ("Registration ownership
// required"). A session must round-trip against the stored row.
func TestIssuedSessionMatchesStoredUserOnNextRequest(t *testing.T) {
	s := &Service{Secret: []byte(strings.Repeat("k", 32)), Lifetime: time.Hour}
	stored := &models.User{ID: 42, Role: "user", Password: "$2y$10$storedbcrypthash"}

	if err := s.IssueSession(httptest.NewRecorder(), &models.User{ID: 42, Role: "user"}); err == nil {
		t.Fatal("session issued from a partial user (no password hash) — it could never validate")
	}

	rec := httptest.NewRecorder()
	if err := s.IssueSession(rec, stored); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/api/v1/register/step2/42", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	claims, err := s.parseSession(req)
	if err != nil {
		t.Fatal(err)
	}
	if !s.sessionMatches(claims, stored) {
		t.Fatal("freshly issued session rejected on the next request")
	}
}

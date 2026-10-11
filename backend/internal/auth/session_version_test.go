package auth

import (
	"signet-backend/internal/models"
	"strings"
	"testing"
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

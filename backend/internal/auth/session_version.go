package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"signet-backend/internal/models"
)

// A keyed credential revision invalidates every old session immediately when
// the persisted password changes, including administrative/reset workflows.
// Neither plaintext password nor its bcrypt hash appears in the JWT.
func (s *Service) credentialVersion(user *models.User) string {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte("session-credential-v1|" + strconv.FormatUint(user.ID, 10) + "|" + user.Password))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) sessionMatches(claims *sessionClaims, user *models.User) bool {
	return claims != nil && user != nil && claims.UserID == user.ID && user.Password != "" && claims.CredentialVersion != "" && hmac.Equal([]byte(claims.CredentialVersion), []byte(s.credentialVersion(user)))
}

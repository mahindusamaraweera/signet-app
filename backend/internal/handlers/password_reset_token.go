package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func newPasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "rt1_" + hex.EncodeToString(bytes), nil
}

// New secrets are hash-only in SQL. Reject submitting a stored digest as a
// bearer token. Legacy links remain accepted only within the enforced expiry.
func passwordResetLookup(token string) (string, error) {
	if token == "" || len(token) > 256 || strings.HasPrefix(token, "sha256:") {
		return "", errResetProof
	}
	if strings.HasPrefix(token, "rt1_") {
		decoded, err := hex.DecodeString(strings.TrimPrefix(token, "rt1_"))
		if err != nil || len(decoded) != 32 {
			return "", errResetProof
		}
		digest := sha256.Sum256([]byte("password-reset-proof-v1|" + token))
		return "sha256:" + hex.EncodeToString(digest[:]), nil
	}
	return token, nil
}

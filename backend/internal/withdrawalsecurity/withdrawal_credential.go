package withdrawalsecurity

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strconv"
)

// Caller already holds the user lock. Persist only a domain-separated keyed
// revision, never a password or its reusable stored bcrypt representation.
func (s *Service) withdrawalCredentialRevision(ctx context.Context, tx *sql.Tx, userID uint64) (string, error) {
	var passwordHash string
	if err := tx.QueryRowContext(ctx, `SELECT password FROM users WHERE id=? FOR UPDATE`, userID).Scan(&passwordHash); err != nil {
		return "", err
	}
	if passwordHash == "" {
		return "", ErrSecurityCredentials
	}
	return withdrawalCredentialDigest(s.key, userID, passwordHash), nil
}

func withdrawalCredentialDigest(key []byte, userID uint64, passwordHash string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("withdrawal-credential-v1|" + strconv.FormatUint(userID, 10) + "|" + passwordHash))
	return hex.EncodeToString(mac.Sum(nil))
}

package registrationverification

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"time"
)

var tokenPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Pending struct {
	ID               string
	Email            string
	VerificationHash string
	Status           string
	ExpiresAt        time.Time
	VerifiedAt       *time.Time
}

func New(key []byte, email string, now time.Time) (Pending, string, error) {
	parsed, err := mail.ParseAddress(email)
	if len(key) < 32 || err != nil || parsed.Address != email {
		return Pending{}, "", errors.New("invalid registration verification configuration")
	}
	id, token := make([]byte, 32), make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		return Pending{}, "", err
	}
	if _, err := rand.Read(token); err != nil {
		return Pending{}, "", err
	}
	// Whole seconds: expires_at is a TIMESTAMP(0) and MySQL ROUNDS fractions
	// on insert, so a sub-second expiry would be read back up to a second
	// later, change ExpiresAt.Unix() in the hash, and break ~half the links.
	p := Pending{ID: hex.EncodeToString(id), Email: email, Status: "pending_delivery", ExpiresAt: now.UTC().Add(24 * time.Hour).Truncate(time.Second)}
	value := hex.EncodeToString(token)
	p.VerificationHash = p.hash(key, value)
	return p, value, nil
}

func (p Pending) hash(key []byte, token string) string {
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "registration-email-v1|%s|%s|%d|%s", p.ID, p.Email, p.ExpiresAt.Unix(), token)
	return hex.EncodeToString(mac.Sum(nil))
}

// Accepts is only the cryptographic gate. The caller must lock and consume
// the pending record atomically with account creation and revalidate referral,
// email/phone uniqueness and registration data before creating any account.
func (p Pending) Accepts(key []byte, token string, now time.Time) bool {
	return len(key) >= 32 && tokenPattern.MatchString(p.ID) && tokenPattern.MatchString(token) &&
		p.Status == "delivered" && p.VerifiedAt == nil && now.Before(p.ExpiresAt) &&
		hmac.Equal([]byte(p.VerificationHash), []byte(p.hash(key, token)))
}

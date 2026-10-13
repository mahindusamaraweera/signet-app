// Package totp implements standard RFC 6238 TOTP generation/verification,
// compatible with Google Authenticator and with the original app's
// sonata-project/google-authenticator PHP library (same algorithm: HMAC-
// SHA1, 30s step, 6 digits, base32 secret).
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// GenerateSecret returns a 32-character base32 secret (160 bits), the same
// size sonata-project/google-authenticator's generateSecret() produces.
func GenerateSecret() string {
	raw := make([]byte, 20)
	_, _ = rand.Read(raw)
	return strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
}

// OtpauthURL builds the otpauth://totp/... URI the QR code encodes, matching
// the original's `signetint:{email}?secret={secret}&issuer=signetint` shape.
func OtpauthURL(issuer, account, secret string) string {
	label := issuer + ":" + account
	query := url.Values{"secret": {secret}, "issuer": {issuer}}
	uri := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + label, RawPath: "/" + url.PathEscape(label), RawQuery: query.Encode()}
	return uri.String()
}

func code(secret string, counter int64) (string, error) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = byte(counter & 0xff)
		counter >>= 8
	}
	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (int(sum[offset])&0x7f)<<24 | (int(sum[offset+1])&0xff)<<16 | (int(sum[offset+2])&0xff)<<8 | (int(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", value%1000000), nil
}

// CheckCode verifies a 6-digit code against the secret, allowing ±1 step
// (30s) of clock drift — the standard tolerance most TOTP apps/libraries
// (including sonata's) use by default.
func CheckCode(secret, submitted string) bool {
	_, ok := MatchingStep(secret, submitted, time.Now(), -1)
	return ok
}

// MatchingStep returns the accepted counter for durable replay protection.
// Callers must atomically persist it with their authorization and reject
// counters at or below lastUsed; this function alone does not consume a code.
func MatchingStep(secret, submitted string, at time.Time, lastUsed int64) (int64, bool) {
	submitted = strings.TrimSpace(submitted)
	if len(submitted) != 6 || secret == "" || at.Unix() < 0 {
		return 0, false
	}
	for _, digit := range submitted {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	now := at.Unix() / 30
	for _, step := range []int64{now + 1, now, now - 1} {
		if step < 0 || step <= lastUsed {
			continue
		}
		expected, err := code(secret, step)
		if err == nil && hmac.Equal([]byte(expected), []byte(submitted)) {
			return step, true
		}
	}
	return 0, false
}

package withdrawalsecurity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type WithdrawalAssertion struct {
	AuthorizationID string `json:"authorization_id"`
	ExternalUserID  string `json:"external_user_id"`
	SecurityVersion string `json:"security_version"`
	AddressID       string `json:"address_id"`
	Destination     string `json:"destination"`
	Network         string `json:"network"`
	Asset           string `json:"asset"`
	Gross           string `json:"gross"`
	Fee             string `json:"fee"`
	Net             string `json:"net"`
	IssuedAt        string `json:"issued_at"`
	ExpiresAt       string `json:"expires_at"`
}

func (a WithdrawalAssertion) payload() string {
	return "signet-external-withdrawal-v1\n" + strings.Join([]string{a.AuthorizationID, a.ExternalUserID, a.SecurityVersion, a.AddressID, a.Destination, a.Network, a.Asset, a.Gross, a.Fee, a.Net, a.IssuedAt, a.ExpiresAt}, "\n")
}

// Worker-only building block. Caller must load consumed approval/outbox and
// revalidate current security under SQL locks before invoking. The separate
// signing key is not the bearer token or browser/session secret. Never expose
// the resulting assertion to the browser; token still checks mapping/replay
// and performs the custody financial audit before any payout.
func signWithdrawalAssertion(a WithdrawalAuthorization, key []byte, now time.Time) (WithdrawalAssertion, string, error) {
	canonical, err := NewWithdrawalTerms(a.Terms.UserID, a.Terms.SecurityVersion, a.Terms.AddressID, a.Terms.Destination, a.Terms.Gross)
	if err != nil || canonical != a.Terms || len(key) < 32 || !challengeIDPattern.MatchString(a.ID) || a.Status != "consumed" || a.AuthorizedAt == nil || a.ConsumedAt == nil || a.ConsumedAt.Before(*a.AuthorizedAt) || a.ConsumedAt.After(now) || !a.AuthorizedAt.Before(a.ExpiresAt) || !a.ConsumedAt.Before(a.ExpiresAt) || now.Unix() <= 0 {
		return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
	}
	t := a.Terms
	assertion := WithdrawalAssertion{AuthorizationID: a.ID, ExternalUserID: strconv.FormatUint(t.UserID, 10), SecurityVersion: strconv.FormatUint(t.SecurityVersion, 10), AddressID: t.AddressID, Destination: t.Destination, Network: t.Network, Asset: t.Asset, Gross: t.Gross, Fee: t.Fee, Net: t.Net, IssuedAt: strconv.FormatInt(now.Unix(), 10), ExpiresAt: strconv.FormatInt(now.Add(time.Minute).Unix(), 10)}
	if a.ExternalUUID != "" {
		if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(a.ExternalUUID) {
			return WithdrawalAssertion{}, "", ErrWithdrawalAuthorization
		}
		assertion.ExternalUserID = a.ExternalUUID
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(assertion.payload()))
	return assertion, hex.EncodeToString(mac.Sum(nil)), nil
}

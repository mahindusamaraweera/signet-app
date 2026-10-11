package totp

import (
	"net/url"
	"testing"
	"time"
)

func TestAuthenticatorURIEncodesLabelAndQuery(t *testing.T) {
	uri, err := url.Parse(OtpauthURL("signet & team", "user+tag?x@example.com", "ABCDEF"))
	if err != nil {
		t.Fatal(err)
	}
	if uri.Query().Get("secret") != "ABCDEF" || uri.Query().Get("issuer") != "signet & team" || uri.Path != "/signet & team:user+tag?x@example.com" {
		t.Fatal("authenticator label corrupted URI parameters")
	}
}

func TestMatchingStepRejectsReplayAndOldCounters(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	at := time.Unix(1234567890, 0)
	now := at.Unix() / 30
	for _, counter := range []int64{now - 1, now, now + 1} {
		value, err := code(secret, counter)
		if err != nil {
			t.Fatal(err)
		}
		matched, ok := MatchingStep(secret, value, at, -1)
		if !ok || matched != counter {
			t.Fatalf("counter %d: matched %d, %v", counter, matched, ok)
		}
		if _, ok := MatchingStep(secret, value, at, counter); ok {
			t.Fatal("reused code accepted")
		}
		if _, ok := MatchingStep(secret, value, at, counter+1); ok {
			t.Fatal("older code accepted")
		}
	}
}

func TestMatchingStepRejectsMalformedAndExpiredCodes(t *testing.T) {
	at := time.Unix(1234567890, 0)
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	for _, value := range []string{"", "12345", "1234567", "12a456", "１２３４５６"} {
		if _, ok := MatchingStep(secret, value, at, -1); ok {
			t.Fatalf("accepted %q", value)
		}
	}
	for _, invalid := range []string{"", "not-base32!"} {
		if _, ok := MatchingStep(invalid, "123456", at, -1); ok {
			t.Fatal("invalid secret accepted")
		}
	}
	old, _ := code(secret, at.Unix()/30-2)
	if _, ok := MatchingStep(secret, old, at, -1); ok {
		t.Fatal("expired code accepted")
	}
}

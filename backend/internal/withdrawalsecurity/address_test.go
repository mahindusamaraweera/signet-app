package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestAddressChallengeBindsDestinationPurposeAndLifecycle(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0)
	address := "0x" + strings.Repeat("a", 40)
	c, code, err := NewAddressEnrollmentChallenge(key, 42, 2, "user@example.com", address, now)
	if err != nil {
		t.Fatal(err)
	}
	if c.AcceptsAddress(key, 42, 2, c.Email, address, code, now) {
		t.Fatal("undelivered accepted")
	}
	c.Status = "delivered"
	if !c.AcceptsAddress(key, 42, 2, c.Email, "0x"+strings.Repeat("A", 40), code, now) {
		t.Fatal("normalized destination rejected")
	}
	if c.AcceptsAddress(key, 42, 2, c.Email, "0x"+strings.Repeat("b", 40), code, now) || c.Accepts(key, 42, 2, c.Email, code, now) || c.AcceptsAddress(key, 43, 2, c.Email, address, code, now) || c.AcceptsAddress(key, 42, 3, c.Email, address, code, now) || c.AcceptsAddress(key, 42, 2, c.Email, address, code, c.ExpiresAt) {
		t.Fatal("altered proof accepted")
	}
	c.ConsumedAt = &now
	if c.AcceptsAddress(key, 42, 2, c.Email, address, code, now) {
		t.Fatal("consumed accepted")
	}
	for _, invalid := range []string{"", "0x" + strings.Repeat("0", 40), "0x" + strings.Repeat("g", 40), address + " "} {
		if _, _, err := NewAddressEnrollmentChallenge(key, 42, 2, c.Email, invalid, now); err == nil {
			t.Fatal("invalid address accepted")
		}
	}
	if !addressEligibleAt(now).Equal(now.Add(24 * time.Hour)) {
		t.Fatal("wrong waiting period")
	}
}

func TestAddressRevocationChallengeCannotAuthorizeAnotherActionOrTarget(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0)
	id := strings.Repeat("a", 64)
	c, code, err := NewAddressRevocationChallenge(key, 42, 2, "user@example.com", id, now)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = "delivered"
	if !c.AcceptsAddressRevocation(key, 42, 2, c.Email, id, code, now) {
		t.Fatal("valid revocation proof rejected")
	}
	if c.AcceptsAddressRevocation(key, 42, 2, c.Email, strings.Repeat("b", 64), code, now) || c.AcceptsAddressRevocation(key, 43, 2, c.Email, id, code, now) || c.AcceptsAddressRevocation(key, 42, 3, c.Email, id, code, now) || c.Accepts(key, 42, 2, c.Email, code, now) || c.AcceptsAddress(key, 42, 2, c.Email, "0x"+strings.Repeat("a", 40), code, now) {
		t.Fatal("cross-target/action proof accepted")
	}
	c.Target = strings.Repeat("b", 64)
	if c.AcceptsAddressRevocation(key, 42, 2, c.Email, c.Target, code, now) {
		t.Fatal("tampered target accepted")
	}
	if _, _, err := NewAddressRevocationChallenge(key, 42, 2, c.Email, "bad", now); err == nil {
		t.Fatal("malformed target accepted")
	}
}

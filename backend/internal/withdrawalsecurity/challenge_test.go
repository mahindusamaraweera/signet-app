package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestEmailChallengeBindsIdentityAndLifecycle(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0)
	c, code, err := NewEmailChallenge(key, 42, 1, "user@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.ID) != 64 || len(code) != 6 || c.Hash == code {
		t.Fatal("invalid challenge generation")
	}
	if c.Accepts(key, 42, 1, c.Email, code, now) {
		t.Fatal("undelivered challenge accepted")
	}
	c.Status = "delivered"
	if !c.Accepts(key, 42, 1, c.Email, code, now) {
		t.Fatal("valid challenge rejected")
	}
	for _, mutation := range []func(*Challenge){
		func(v *Challenge) { v.UserID++ }, func(v *Challenge) { v.SecurityVersion++ },
		func(v *Challenge) { v.Email = "changed@example.com" }, func(v *Challenge) { v.Purpose = "withdrawal" },
		func(v *Challenge) { v.Attempts = 5 }, func(v *Challenge) { v.ConsumedAt = &now },
		func(v *Challenge) { v.ExpiresAt = now }, func(v *Challenge) { v.ID = strings.Repeat("a", 64) },
	} {
		copy := c
		mutation(&copy)
		if copy.Accepts(key, 42, 1, c.Email, code, now) {
			t.Fatal("changed challenge accepted")
		}
	}
	if c.Accepts([]byte(strings.Repeat("x", 32)), 42, 1, c.Email, code, now) {
		t.Fatal("wrong key accepted")
	}
	if c.Accepts(key, 42, 1, c.Email, "12x456", now) {
		t.Fatal("malformed code accepted")
	}
	if _, _, err := NewEmailChallenge([]byte("weak"), 42, 1, c.Email, now); err == nil {
		t.Fatal("weak key accepted")
	}
}

func TestDeviceChallengeCannotAuthorizeAnotherDeviceOrEmailVerification(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0)
	target := strings.Repeat("a", 64)
	c, code, err := NewDeviceEnrollmentChallenge(key, 42, 1, "user@example.com", target, now)
	if err != nil {
		t.Fatal(err)
	}
	c.Status = "delivered"
	if !c.acceptsAction(key, 42, 1, c.Email, code, now, DeviceEnrollmentPurpose, target) {
		t.Fatal("valid device proof rejected")
	}
	if c.acceptsAction(key, 42, 1, c.Email, code, now, DeviceEnrollmentPurpose, strings.Repeat("b", 64)) || c.Accepts(key, 42, 1, c.Email, code, now) {
		t.Fatal("cross-device/purpose proof accepted")
	}
	c.Target = strings.Repeat("b", 64)
	if c.acceptsAction(key, 42, 1, c.Email, code, now, DeviceEnrollmentPurpose, c.Target) {
		t.Fatal("altered target accepted")
	}
	if _, _, err := NewDeviceEnrollmentChallenge(key, 42, 1, c.Email, "bad-target", now); err == nil {
		t.Fatal("invalid target accepted")
	}
}

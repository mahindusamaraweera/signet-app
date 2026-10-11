package registrationverification

import (
	"strings"
	"testing"
	"time"
)

func TestRegistrationTokenRequiresDeliveredUnexpiredUnusedProof(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0)
	p, token, err := New(key, "user@example.com", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 64 || p.VerificationHash == token {
		t.Fatal("unsafe token")
	}
	if p.Accepts(key, token, now) {
		t.Fatal("undelivered proof accepted")
	}
	p.Status = "delivered"
	if !p.Accepts(key, token, now) {
		t.Fatal("valid proof rejected")
	}
	for _, mutate := range []func(*Pending){func(v *Pending) { v.Email = "other@example.com" }, func(v *Pending) { v.ID = strings.Repeat("f", 64) },
		func(v *Pending) { v.VerifiedAt = &now }, func(v *Pending) { v.Status = "superseded" }, func(v *Pending) { v.ExpiresAt = now }} {
		copy := p
		mutate(&copy)
		if copy.Accepts(key, token, now) {
			t.Fatal("changed proof accepted")
		}
	}
	if p.Accepts(key, "bad", now) || p.Accepts(key, token, p.ExpiresAt) {
		t.Fatal("invalid/expired proof accepted")
	}
	if _, _, err := New([]byte("weak"), p.Email, now); err == nil {
		t.Fatal("weak key accepted")
	}
}

package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestWithdrawalEmailProofBindsPreparedRequestAndTerms(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := NewWithdrawalAuthorization(key, terms, now)
	if err != nil {
		t.Fatal(err)
	}
	c, code, err := NewWithdrawalChallenge(key, "user@example.com", a, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if c.ExpiresAt != a.ExpiresAt || c.AcceptsWithdrawal(key, c.Email, code, a, now) {
		t.Fatal("expiry extended or undelivered accepted")
	}
	c.Status = "delivered"
	if !c.AcceptsWithdrawal(key, c.Email, code, a, now.Add(time.Minute)) || c.Accepts(key, 42, 2, c.Email, code, now) {
		t.Fatal("valid proof rejected or setup reuse accepted")
	}
	for _, mutate := range []func(*WithdrawalAuthorization){func(a *WithdrawalAuthorization) { a.ID = strings.Repeat("c", 64) }, func(a *WithdrawalAuthorization) { a.Terms.UserID++ }, func(a *WithdrawalAuthorization) { a.Terms.SecurityVersion++ }, func(a *WithdrawalAuthorization) { a.Terms.Gross = "101.000000000000000000" }, func(a *WithdrawalAuthorization) { a.Status = "authorized" }, func(a *WithdrawalAuthorization) { a.ConsumedAt = &now }} {
		copy := a
		mutate(&copy)
		if c.AcceptsWithdrawal(key, c.Email, code, copy, now) {
			t.Fatal("changed request accepted")
		}
	}
	if c.AcceptsWithdrawal(key, "changed@example.com", code, a, now) || c.AcceptsWithdrawal(key, c.Email, code, a, a.ExpiresAt) {
		t.Fatal("changed email or expired accepted")
	}
	if _, _, err := NewWithdrawalChallenge(key, c.Email, a, a.ExpiresAt); err == nil {
		t.Fatal("expired request got new code")
	}
}

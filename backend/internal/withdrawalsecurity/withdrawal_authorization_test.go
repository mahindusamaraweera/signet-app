package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestWithdrawalAuthorizationBindsTermsAndSingleUseLifecycle(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("a", 64), "0x"+strings.Repeat("b", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	a, proof, err := NewWithdrawalAuthorization(key, terms, now)
	if err != nil {
		t.Fatal(err)
	}
	if a.ProofHash == proof || a.CanConsume(key, proof, terms, now) {
		t.Fatal("pending proof authorized")
	}
	a.Status = "authorized"
	a.AuthorizedAt = &now
	if !a.CanConsume(key, proof, terms, now) {
		t.Fatal("valid authorized proof rejected")
	}
	for _, mutate := range []func(*WithdrawalAuthorization){func(a *WithdrawalAuthorization) { a.ID = strings.Repeat("c", 64) }, func(a *WithdrawalAuthorization) { a.Terms.Gross = "101" }, func(a *WithdrawalAuthorization) { a.Terms.SecurityVersion++ }, func(a *WithdrawalAuthorization) { a.ExpiresAt = a.ExpiresAt.Add(time.Second) }, func(a *WithdrawalAuthorization) { a.Status = "consumed" }, func(a *WithdrawalAuthorization) { a.ConsumedAt = &now }, func(a *WithdrawalAuthorization) { a.AuthorizedAt = nil }} {
		copy := a
		mutate(&copy)
		if copy.CanConsume(key, proof, terms, now) {
			t.Fatal("altered authorization accepted")
		}
	}
	if a.CanConsume(key, proof, terms, a.ExpiresAt) || a.CanConsume([]byte(strings.Repeat("x", 32)), proof, terms, now) || a.CanConsume(key, strings.Repeat("c", 64), terms, now) {
		t.Fatal("expired or wrong proof accepted")
	}
	terms.Fee = "4.000000000000000000"
	if _, _, err := NewWithdrawalAuthorization(key, terms, now); err == nil {
		t.Fatal("inconsistent fee authorized")
	}
}

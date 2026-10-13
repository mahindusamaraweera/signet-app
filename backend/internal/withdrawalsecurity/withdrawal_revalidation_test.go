package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestFreshWithdrawalProofUsesShortLifetimeAndDistinctChallengeDomain(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	key := []byte(strings.Repeat("k", 32))
	terms, _ := NewWithdrawalTerms(42, 2, strings.Repeat("b", 64), "0x"+strings.Repeat("c", 40), "100")
	a := WithdrawalAuthorization{ID: strings.Repeat("a", 64), Terms: terms, Status: "consumed", ExpiresAt: now.Add(time.Minute), AuthorizedAt: &now, ConsumedAt: &now}
	p, err := signWithdrawalRevalidation(a, 7, strings.Repeat("e", 64), key, now)
	if err != nil || p.Assertion.ExpiresAt != "1800000015" || p.RemoteWithdrawalID != "7" {
		t.Fatalf("invalid fresh proof: %v", err)
	}
	_, reservationSignature, _ := signWithdrawalAssertion(a, key, now)
	changedNonce, _ := signWithdrawalRevalidation(a, 7, strings.Repeat("f", 64), key, now)
	changedRemote, _ := signWithdrawalRevalidation(a, 8, strings.Repeat("e", 64), key, now)
	if p.Signature == reservationSignature || p.Signature == changedNonce.Signature || p.Signature == changedRemote.Signature {
		t.Fatal("fresh proof is not challenge-bound")
	}
	if _, err := signWithdrawalRevalidation(a, 0, strings.Repeat("e", 64), key, now); err == nil {
		t.Fatal("missing remote identity signed")
	}
	if _, err := signWithdrawalRevalidation(a, 7, "weak", key, now); err == nil {
		t.Fatal("invalid nonce signed")
	}
}

package withdrawalsecurity

import (
	"strings"
	"testing"
	"time"
)

func TestWithdrawalAssertionMatchesTokenWireFormat(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	terms, err := NewWithdrawalTerms(42, 2, strings.Repeat("b", 64), "0x"+strings.Repeat("c", 40), "100")
	if err != nil {
		t.Fatal(err)
	}
	a := WithdrawalAuthorization{ID: strings.Repeat("a", 64), Terms: terms, Status: "consumed", ExpiresAt: now.Add(10 * time.Minute), AuthorizedAt: &now, ConsumedAt: &now}
	assertion, signature, err := signWithdrawalAssertion(a, []byte(strings.Repeat("k", 32)), now)
	if err != nil || assertion.IssuedAt != "1800000000" || assertion.ExpiresAt != "1800000060" || len(signature) != 64 {
		t.Fatalf("invalid assertion: %v", err)
	}
	expected := "signet-external-withdrawal-v1\n" + strings.Repeat("a", 64) + "\n42\n2\n" + strings.Repeat("b", 64) + "\n0x" + strings.Repeat("c", 40) + "\nBSC\nSIGNET\n100.000000000000000000\n3.000000000000000000\n97.000000000000000000\n1800000000\n1800000060"
	if assertion.payload() != expected {
		t.Fatal("token wire format mismatch")
	}
	if signature != "605ba1ba8af1a37b132acae1c14f2a3643182a73fc1a239eed8ad7cc5f91da6c" {
		t.Fatal("PHP/Go signature mismatch")
	}
	for _, state := range []string{"pending", "authorized", "invalidated"} {
		copy := a
		copy.Status = state
		if _, _, err := signWithdrawalAssertion(copy, []byte(strings.Repeat("k", 32)), now); err == nil {
			t.Fatal("unconsumed approval signed")
		}
	}
	if _, _, err := signWithdrawalAssertion(a, []byte("weak"), now); err == nil {
		t.Fatal("weak key signed")
	}
}

func TestWithdrawalAssertionsBindCustomerUUIDWithoutChangingLocalApprovalTerms(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	terms, _ := NewWithdrawalTerms(461, 2, strings.Repeat("b", 64), "0x"+strings.Repeat("c", 40), "100")
	a := WithdrawalAuthorization{ExternalUUID: "01a13b42-28d0-7000-8000-0000000001cd", ID: strings.Repeat("a", 64), Terms: terms, Status: "consumed", ExpiresAt: now.Add(time.Hour), AuthorizedAt: &now, ConsumedAt: &now}
	assertion, signature, err := signWithdrawalAssertion(a, []byte(strings.Repeat("k", 32)), now)
	if err != nil || assertion.ExternalUserID != a.ExternalUUID || terms.UserID != 461 {
		t.Fatal("UUID assertion lost approval owner", err)
	}
	if signature != "1a7091cefc180172d67dc964a205d98edf51da214c7d7665ab336d5009034caf" {
		t.Fatal("UUID signature differs from PHP contract vector")
	}
	other := a
	other.ExternalUUID = "11111111-1111-4111-8111-111111111111"
	_, otherSignature, err := signWithdrawalAssertion(other, []byte(strings.Repeat("k", 32)), now)
	if err != nil || otherSignature == signature {
		t.Fatal("signature was not bound to UUID", err)
	}
}

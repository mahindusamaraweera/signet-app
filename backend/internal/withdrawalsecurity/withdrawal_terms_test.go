package withdrawalsecurity

import (
	"strings"
	"testing"
)

func TestWithdrawalTermsDeductFeeOnceAndBindTerms(t *testing.T) {
	id := strings.Repeat("a", 64)
	address := "0x" + strings.Repeat("b", 40)
	terms, err := NewWithdrawalTerms(42, 2, id, address, "100")
	if err != nil || terms.Gross != "100.000000000000000000" || terms.Fee != "3.000000000000000000" || terms.Net != "97.000000000000000000" {
		t.Fatalf("wrong fee split: %+v %v", terms, err)
	}
	same, err := NewWithdrawalTerms(42, 2, id, address, "100.000")
	if err != nil || same.Digest() != terms.Digest() {
		t.Fatal("equivalent amount changed binding")
	}
	for _, mutate := range []func(*WithdrawalTerms){func(t *WithdrawalTerms) { t.UserID++ }, func(t *WithdrawalTerms) { t.SecurityVersion++ }, func(t *WithdrawalTerms) { t.Destination = "0x" + strings.Repeat("c", 40) }, func(t *WithdrawalTerms) { t.Gross = "101" }, func(t *WithdrawalTerms) { t.Fee = "4" }, func(t *WithdrawalTerms) { t.Net = "96" }, func(t *WithdrawalTerms) { t.Asset = "USDT" }, func(t *WithdrawalTerms) { t.AddressID = strings.Repeat("c", 64) }} {
		copy := terms
		mutate(&copy)
		if copy.Digest() == terms.Digest() {
			t.Fatal("changed terms retained digest")
		}
	}
	tiny, err := NewWithdrawalTerms(42, 2, id, address, "0.000000000000000101")
	if err != nil || tiny.Fee != "0.000000000000000003" || tiny.Net != "0.000000000000000098" {
		t.Fatal("fee must truncate at 18 decimals")
	}
	for _, invalid := range []string{"0", "-1", "1e2", "1.0000000000000000001", " 100", "100 ", "NaN"} {
		if _, err := NewWithdrawalTerms(42, 2, id, address, invalid); err == nil {
			t.Fatal("invalid amount accepted")
		}
	}
}

package blockchain

import (
	"fmt"
	"testing"
)

func TestCommercialPackageCatalog(t *testing.T) {
	for _, tc := range []struct{ price, fee, bonus int64 }{{1000, 130, 400}, {2500, 280, 500}, {5000, 1030, 600}, {10000, 2030, 700}, {25000, 5030, 800}, {50000, 15030, 900}, {100000, 30030, 1000}, {500000, 150030, 1100}, {1000000, 400030, 1200}, {500, 80, 200}, {100, 20, 100}} {
		terms, err := CommercialPackageTerms(tc.price)
		if err != nil || terms.Fee != fmt.Sprint(tc.fee) || terms.BonusPercent != tc.bonus || terms.Required != fmt.Sprint(tc.price+tc.fee) {
			t.Fatalf("incorrect package %d: %+v %v", tc.price, terms, err)
		}
	}
	if _, err := CommercialPackageTerms(2000); err == nil {
		t.Fatal("unsupported package accepted")
	}
}

func TestLiveTestPackageExactDecimalTerms(t *testing.T) {
	terms, err := CommercialPackageTerms(1)
	if err != nil || terms.Fee != "0.10" || terms.Required != "1.10" || terms.BonusPercent != 200 {
		t.Fatalf("wrong test terms: %+v %v", terms, err)
	}
}

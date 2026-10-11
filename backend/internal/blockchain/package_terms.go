package blockchain

import "fmt"

type PackageTerms struct {
	Fee          string
	BonusPercent int64
	Required     string
}

func CommercialPackageTerms(price int64) (PackageTerms, error) {
	catalog := map[int64]PackageTerms{
		1:   {Fee: "0.10", Required: "1.10", BonusPercent: 200},
		100: {Fee: "20", BonusPercent: 100}, 500: {Fee: "80", BonusPercent: 200}, 1000: {Fee: "130", BonusPercent: 400},
		2500: {Fee: "280", BonusPercent: 500}, 5000: {Fee: "1030", BonusPercent: 600}, 10000: {Fee: "2030", BonusPercent: 700},
		25000: {Fee: "5030", BonusPercent: 800}, 50000: {Fee: "15030", BonusPercent: 900}, 100000: {Fee: "30030", BonusPercent: 1000},
		500000: {Fee: "150030", BonusPercent: 1100}, 1000000: {Fee: "400030", BonusPercent: 1200},
	}
	terms, ok := catalog[price]
	if !ok {
		return PackageTerms{}, fmt.Errorf("package price is not in the approved commercial catalog")
	}
	if terms.Required == "" {
		var fee int64
		if _, err := fmt.Sscan(terms.Fee, &fee); err != nil {
			return PackageTerms{}, err
		}
		terms.Required = fmt.Sprint(price + fee)
	}
	return terms, nil
}

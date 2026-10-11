package wallet

import "testing"

func TestCommissionFitsCapLegacyBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		balance, commission, cap float64
		allowed                  bool
	}{
		{"below", 300, 50, 400, true},
		{"equal", 350, 50, 400, true},
		{"above", 351, 50, 400, false},
		{"already above", 401, 0, 400, false},
		{"no active package", 0, 50, 0, false},
		{"zero at zero", 0, 0, 0, true},
		{"multiple packages sum", 1150, 50, 1200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CommissionFitsCap(tc.balance, tc.commission, tc.cap); got != tc.allowed {
				t.Fatalf("allowed=%v, want %v", got, tc.allowed)
			}
		})
	}
}

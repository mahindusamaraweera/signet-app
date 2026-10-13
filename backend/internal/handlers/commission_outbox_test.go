package handlers

import "testing"

func TestCommissionOutboxRejectsInvalidEvidenceBeforeDatabaseUse(t *testing.T) {
	for _, amount := range []string{"-1", "NaN", "1/2", "1e3", "1000000000000000000", "0.1234567890123456789"} {
		if err := purchaseCommissionRecorder(1)(nil, 2, 3, amount, ""); err == nil {
			t.Fatalf("accepted invalid amount %q", amount)
		}
	}
	for _, ids := range [][3]uint64{{0, 2, 3}, {1, 0, 3}, {1, 2, 0}} {
		if err := purchaseCommissionRecorder(ids[0])(nil, int64(ids[1]), ids[2], "50", ""); err == nil {
			t.Fatal("accepted missing evidence identity")
		}
	}
}

func TestZeroCommissionDoesNotCreatePayoutEvidence(t *testing.T) {
	if err := purchaseCommissionRecorder(1)(nil, 2, 3, "0.000000", ""); err != nil {
		t.Fatal(err)
	}
}

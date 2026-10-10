package blockchain

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

type backfillFake struct {
	calls   []uint64
	fail    uint64
	pending uint64
}

func (f *backfillFake) EnsureWallet(_ context.Context, id uint64) (*Wallet, error) {
	f.calls = append(f.calls, id)
	if id == f.fail {
		return nil, errors.New("service unavailable")
	}
	status := "ready"
	if id == f.pending {
		status = "pending"
	}
	return &Wallet{ProvisioningStatus: status, Address: "0x1234567890123456789012345678901234567890", ChainID: 97}, nil
}
func TestBackfillDryRunNeverProvisions(t *testing.T) {
	service := &backfillFake{}
	cursor, err := BackfillWallets(context.Background(), []uint64{2, 5}, false, service, &bytes.Buffer{})
	if err != nil || cursor != 5 || len(service.calls) != 0 {
		t.Fatalf("cursor=%d calls=%v err=%v", cursor, service.calls, err)
	}
}
func TestBackfillStopsBeforeFailedOrPendingUser(t *testing.T) {
	for _, service := range []*backfillFake{{fail: 5}, {pending: 5}} {
		cursor, err := BackfillWallets(context.Background(), []uint64{2, 5, 9}, true, service, &bytes.Buffer{})
		if err == nil || cursor != 2 || len(service.calls) != 2 {
			t.Fatalf("cursor=%d calls=%v err=%v", cursor, service.calls, err)
		}
	}
}

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
	return &Wallet{ProvisioningStatus: status, Address: "0x1234567890123456789012345678901234567890", ChainID: 56, Asset: "USDT"}, nil
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

type walletProvisionFunc func(context.Context, uint64) (*Wallet, error)

func (f walletProvisionFunc) EnsureWallet(ctx context.Context, id uint64) (*Wallet, error) {
	return f(ctx, id)
}

type walletAuditFake struct {
	calls []uint64
	fail  uint64
}

func (a *walletAuditFake) RecordWallet(_ context.Context, id uint64, _ *Wallet) error {
	a.calls = append(a.calls, id)
	if id == a.fail {
		return errors.New("audit unavailable")
	}
	return nil
}
func TestBackfillAuditFailureNeverAdvancesCursor(t *testing.T) {
	service := &backfillFake{}
	audit := &walletAuditFake{fail: 5}
	cursor, err := BackfillWalletsAudited(context.Background(), []uint64{2, 5, 9}, true, 56, service, audit, &bytes.Buffer{})
	if err == nil || cursor != 2 || len(service.calls) != 2 || len(audit.calls) != 2 {
		t.Fatalf("unsafe audit cursor=%d calls=%v err=%v", cursor, audit.calls, err)
	}
}
func TestBackfillDryRunNeverAudits(t *testing.T) {
	service := &backfillFake{}
	audit := &walletAuditFake{fail: 2}
	cursor, err := BackfillWalletsAudited(context.Background(), []uint64{2, 5}, false, 56, service, audit, &bytes.Buffer{})
	if err != nil || cursor != 5 || len(service.calls) != 0 || len(audit.calls) != 0 {
		t.Fatal("dry run changed custody or audit")
	}
}
func TestBackfillRejectsInvalidWalletBeforeAudit(t *testing.T) {
	for _, scenario := range []string{"wrong chain", "wrong asset", "zero address", "malformed address"} {
		t.Run(scenario, func(t *testing.T) {
			service := walletProvisionFunc(func(context.Context, uint64) (*Wallet, error) {
				w := &Wallet{ProvisioningStatus: "ready", Address: "0x1234567890123456789012345678901234567890", ChainID: 56, Asset: "USDT"}
				switch scenario {
				case "wrong chain":
					w.ChainID = 97
				case "wrong asset":
					w.Asset = "SIGNET"
				case "zero address":
					w.Address = "0x0000000000000000000000000000000000000000"
				case "malformed address":
					w.Address = "0xnot-an-address"
				}
				return w, nil
			})
			audit := &walletAuditFake{}
			cursor, err := BackfillWalletsAudited(context.Background(), []uint64{2}, true, 56, service, audit, &bytes.Buffer{})
			if err == nil || cursor != 0 || len(audit.calls) != 0 {
				t.Fatal("invalid wallet was recorded")
			}
		})
	}
}
func TestBackfillTestnetNeedsExplicitNetwork(t *testing.T) {
	service := walletProvisionFunc(func(context.Context, uint64) (*Wallet, error) {
		return &Wallet{ProvisioningStatus: "ready", Address: "0x1234567890123456789012345678901234567890", ChainID: 97, Asset: "USDT"}, nil
	})
	if cursor, err := BackfillWalletsAudited(context.Background(), []uint64{2}, true, 97, service, nil, &bytes.Buffer{}); err != nil || cursor != 2 {
		t.Fatalf("explicit testnet failed: %v", err)
	}
}
func TestBackfillValidatesEntireBatchBeforeProvisioning(t *testing.T) {
	for _, ids := range [][]uint64{{2, 2}, {2, 0}, {5, 2}} {
		service := &backfillFake{}
		cursor, err := BackfillWallets(context.Background(), ids, true, service, &bytes.Buffer{})
		if err == nil || cursor != 0 || len(service.calls) != 0 {
			t.Fatal("invalid batch provisioned a wallet")
		}
	}
}

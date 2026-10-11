package blockchain

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

type WalletProvisioner interface {
	EnsureWallet(context.Context, uint64) (*Wallet, error)
}

type WalletAuditRecorder interface {
	RecordWallet(context.Context, uint64, *Wallet) error
}

// BackfillWallets never changes source accounts or imports financial allocations.
// The returned cursor advances only over ready wallets, or reviewed IDs in dry-run mode.
func BackfillWallets(ctx context.Context, ids []uint64, apply bool, service WalletProvisioner, out io.Writer) (uint64, error) {
	return BackfillWalletsAudited(ctx, ids, apply, 56, service, nil, out)
}

// Audited provisioning never enrolls users in expiry or imports balances/vesting.
// An audit failure retains the last successful cursor; retry the same identity.
func BackfillWalletsAudited(ctx context.Context, ids []uint64, apply bool, chainID uint64, service WalletProvisioner, audit WalletAuditRecorder, out io.Writer) (uint64, error) {
	var cursor uint64
	if out == nil || (chainID != 56 && chainID != 97) {
		return 0, fmt.Errorf("BSC mainnet/testnet chain and output required")
	}
	var previous uint64
	for _, id := range ids {
		if id == 0 || id <= previous {
			return 0, fmt.Errorf("user IDs must be positive and ascending")
		}
		previous = id
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return cursor, err
		}
		if id == 0 || id <= cursor {
			return cursor, fmt.Errorf("user IDs must be positive and ascending")
		}
		if !apply {
			if _, err := fmt.Fprintf(out, "dry-run user_id=%d\n", id); err != nil {
				return cursor, err
			}
			cursor = id
			continue
		}
		if service == nil {
			return cursor, fmt.Errorf("wallet service required")
		}
		wallet, err := service.EnsureWallet(ctx, id)
		if err != nil {
			return cursor, fmt.Errorf("provisioning user %d failed: %w", id, err)
		}
		if wallet == nil || wallet.ProvisioningStatus != "ready" {
			return cursor, fmt.Errorf("user %d provisioning is pending; retry this user", id)
		}
		address, decodeErr := hex.DecodeString(strings.TrimPrefix(wallet.Address, "0x"))
		if wallet.ChainID != chainID || wallet.Asset != "USDT" || !strings.HasPrefix(wallet.Address, "0x") || len(wallet.Address) != 42 || decodeErr != nil || len(address) != 20 || strings.Trim(wallet.Address[2:], "0") == "" {
			return cursor, fmt.Errorf("user %d wallet network, asset or deposit address is invalid", id)
		}
		if audit != nil {
			auditCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := audit.RecordWallet(auditCtx, id, wallet)
			cancel()
			if err != nil {
				return cursor, fmt.Errorf("user %d wallet audit failed; retry this user: %w", id, err)
			}
		}
		if _, err := fmt.Fprintf(out, "ready user_id=%d address=%s chain_id=%d\n", id, wallet.Address, wallet.ChainID); err != nil {
			return cursor, err
		}
		cursor = id
	}
	return cursor, nil
}

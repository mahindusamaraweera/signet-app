package blockchain

import (
	"context"
	"fmt"
	"io"
)

type WalletProvisioner interface {
	EnsureWallet(context.Context, uint64) (*Wallet, error)
}

// BackfillWallets never changes source accounts or imports financial allocations.
// The returned cursor advances only over ready wallets, or reviewed IDs in dry-run mode.
func BackfillWallets(ctx context.Context, ids []uint64, apply bool, service WalletProvisioner, out io.Writer) (uint64, error) {
	var cursor uint64
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
		if _, err := fmt.Fprintf(out, "ready user_id=%d address=%s chain_id=%d\n", id, wallet.Address, wallet.ChainID); err != nil {
			return cursor, err
		}
		cursor = id
	}
	return cursor, nil
}

package blockchain

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// SQLWalletAuditRecorder only writes the independent provisioning audit table.
// A retry may confirm an existing mapping but may never replace its address.
type SQLWalletAuditRecorder struct{ DB *sql.DB }

func (r SQLWalletAuditRecorder) RecordWallet(ctx context.Context, userID uint64, wallet *Wallet) error {
	if r.DB == nil || userID == 0 || wallet == nil {
		return fmt.Errorf("wallet audit configuration required")
	}
	address := strings.ToLower(wallet.Address)
	if _, err := r.DB.ExecContext(ctx, `INSERT INTO blockchain_wallet_provisioning_audit (source_user_id,chain_id,deposit_address) VALUES (?,?,?) ON DUPLICATE KEY UPDATE source_user_id=source_user_id`, userID, wallet.ChainID, address); err != nil {
		return err
	}
	var recordedChain uint64
	var recordedAddress string
	if err := r.DB.QueryRowContext(ctx, `SELECT chain_id,deposit_address FROM blockchain_wallet_provisioning_audit WHERE source_user_id=?`, userID).Scan(&recordedChain, &recordedAddress); err != nil {
		return err
	}
	if recordedChain != wallet.ChainID || recordedAddress != address {
		return fmt.Errorf("existing wallet audit mapping differs; reconciliation required")
	}
	return nil
}

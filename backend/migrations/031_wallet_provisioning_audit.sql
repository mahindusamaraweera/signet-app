-- Independent append-only mapping evidence; never update users or enroll old
-- users in registration expiry. Keep this record after account deletion.
CREATE TABLE IF NOT EXISTS blockchain_wallet_provisioning_audit (
  source_user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  chain_id BIGINT UNSIGNED NOT NULL,
  deposit_address VARCHAR(42) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  provisioned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY wallet_audit_chain_address (chain_id,deposit_address)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

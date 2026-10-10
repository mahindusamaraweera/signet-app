CREATE TABLE IF NOT EXISTS withdrawal_security_addresses (
  id CHAR(64) NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  network VARCHAR(16) NOT NULL,
  asset VARCHAR(16) NOT NULL,
  address CHAR(42) NOT NULL,
  security_version BIGINT UNSIGNED NOT NULL,
  verified_at TIMESTAMP NOT NULL,
  eligible_at TIMESTAMP NOT NULL,
  revoked_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY withdrawal_address_owner_destination (user_id, network, asset, address),
  KEY withdrawal_address_owner (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

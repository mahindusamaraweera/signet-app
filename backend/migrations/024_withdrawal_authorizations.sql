CREATE TABLE IF NOT EXISTS withdrawal_authorizations (
  id CHAR(64) NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  security_version BIGINT UNSIGNED NOT NULL,
  address_id CHAR(64) NOT NULL,
  terms JSON NOT NULL,
  terms_digest CHAR(64) NOT NULL,
  proof_hash CHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMP NOT NULL,
  authorized_at TIMESTAMP NULL,
  consumed_at TIMESTAMP NULL,
  remote_withdrawal_id BIGINT UNSIGNED NULL,
  created_at TIMESTAMP NOT NULL,
  KEY withdrawal_authorization_owner (user_id,created_at),
  KEY withdrawal_authorization_pending (status,expires_at),
  UNIQUE KEY withdrawal_authorization_remote (remote_withdrawal_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

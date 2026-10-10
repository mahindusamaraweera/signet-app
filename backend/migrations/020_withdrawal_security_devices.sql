CREATE TABLE IF NOT EXISTS withdrawal_security_devices (
  id CHAR(64) NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  credential_hash CHAR(64) NOT NULL,
  security_version BIGINT UNSIGNED NULL,
  verified_at TIMESTAMP NULL,
  eligible_at TIMESTAMP NULL,
  revoked_at TIMESTAMP NULL,
  last_seen_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY withdrawal_device_credential (user_id, credential_hash),
  KEY withdrawal_device_owner (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

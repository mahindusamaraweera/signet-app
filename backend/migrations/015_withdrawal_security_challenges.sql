-- Withdrawal-only security evidence; existing MLM rules are unchanged.
CREATE TABLE IF NOT EXISTS withdrawal_security_profiles (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  verified_email VARCHAR(255) NULL,
  email_verified_at TIMESTAMP NULL,
  authenticator_fingerprint CHAR(64) NULL,
  authenticator_confirmed_at TIMESTAMP NULL,
  security_version BIGINT UNSIGNED NOT NULL DEFAULT 1,
  withdrawals_frozen TINYINT(1) NOT NULL DEFAULT 0,
  hold_until TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS withdrawal_security_challenges (
  id CHAR(64) NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  purpose VARCHAR(32) NOT NULL,
  email VARCHAR(255) NOT NULL,
  code_hash CHAR(64) NOT NULL,
  security_version BIGINT UNSIGNED NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending_delivery',
  attempts TINYINT UNSIGNED NOT NULL DEFAULT 0,
  expires_at TIMESTAMP NOT NULL,
  consumed_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY withdrawal_challenge_user (user_id, purpose, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

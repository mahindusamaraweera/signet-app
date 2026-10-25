-- Read-only mirror of Token's linked Tier verification; legacy kycs remain intact.
CREATE TABLE IF NOT EXISTS tier_identity_verifications (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  tier_user_id VARCHAR(64) NOT NULL,
  status VARCHAR(40) NOT NULL,
  verified TINYINT(1) NOT NULL DEFAULT 0,
  environment VARCHAR(20) NOT NULL,
  level_name VARCHAR(100) NOT NULL,
  provider_checked_at DATETIME(6) NOT NULL,
  valid_until DATETIME(6) NULL,
  synced_at DATETIME(6) NOT NULL,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY tier_identity_refresh (synced_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

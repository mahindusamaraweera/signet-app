CREATE TABLE IF NOT EXISTS tier_account_link_requests (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  state_hash CHAR(64) NOT NULL,
  session_hash CHAR(64) NOT NULL,
  verifier_encrypted TEXT NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending',
  expires_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TIMESTAMP NULL,
  UNIQUE KEY tier_link_state_unique (state_hash),
  KEY tier_link_request_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS tier_account_links (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  tier_user_id VARCHAR(64) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY tier_link_source_unique (user_id),
  UNIQUE KEY tier_link_destination_unique (tier_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS package_purchase_intents (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  user_package_id BIGINT UNSIGNED NOT NULL,
  package_price DECIMAL(36,18) NOT NULL,
  commission_user_id BIGINT UNSIGNED NOT NULL,
  commission_percent BIGINT NOT NULL DEFAULT 0,
  status VARCHAR(40) NOT NULL DEFAULT 'awaiting_deposit',
  available_usdt DECIMAL(36,18) NOT NULL DEFAULT 0,
  remaining_usdt DECIMAL(36,18) NOT NULL,
  deposit_seen BOOLEAN NOT NULL DEFAULT FALSE,
  remote_order_id VARCHAR(100) NULL,
  last_checked_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY purchase_intent_package_unique (user_package_id),
  KEY purchase_intent_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

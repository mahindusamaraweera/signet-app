CREATE TABLE IF NOT EXISTS blockchain_commission_outbox (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  purchase_intent_id BIGINT UNSIGNED NOT NULL,
  earn_log_id BIGINT UNSIGNED NOT NULL,
  beneficiary_user_id BIGINT UNSIGNED NOT NULL,
  commission_usd DECIMAL(36,18) NOT NULL,
  description VARCHAR(255) NOT NULL DEFAULT '',
  status VARCHAR(40) NOT NULL DEFAULT 'awaiting_policy',
  remote_payment_id VARCHAR(100) NULL,
  signet_price_usdt DECIMAL(36,18) NULL,
  signet_amount DECIMAL(36,18) NULL,
  transfer_tx_hash VARCHAR(128) NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY commission_earn_log_unique (earn_log_id),
  KEY commission_intent (purchase_intent_id),
  KEY commission_pending (status,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS income_cap_diversions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  purchase_intent_id BIGINT UNSIGNED NOT NULL,
  commission_usd DECIMAL(36,18) NOT NULL,
  company_user_id BIGINT UNSIGNED NOT NULL DEFAULT 1,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY cap_diversion_identity (user_id,purchase_intent_id),
  KEY cap_diversion_user (user_id,id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

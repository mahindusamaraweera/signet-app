CREATE TABLE IF NOT EXISTS registration_reopening_outbox (
  purchase_intent_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  expired_placement_log_id BIGINT UNSIGNED NOT NULL,
  new_placement_log_id BIGINT UNSIGNED NOT NULL,
  status VARCHAR(40) NOT NULL DEFAULT 'pending',
  attempts BIGINT UNSIGNED NOT NULL DEFAULT 0,
  last_error VARCHAR(200) NULL,
  last_attempt_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  confirmed_at TIMESTAMP NULL,
  UNIQUE KEY reopening_expired_placement (expired_placement_log_id),
  UNIQUE KEY reopening_new_placement (new_placement_log_id),
  KEY reopening_retry (status, created_at),
  CONSTRAINT reopening_purchase FOREIGN KEY (purchase_intent_id) REFERENCES package_purchase_intents(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

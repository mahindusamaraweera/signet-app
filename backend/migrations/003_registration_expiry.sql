CREATE TABLE IF NOT EXISTS registration_expiries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  placement_log_id BIGINT UNSIGNED NOT NULL,
  user_id BIGINT UNSIGNED NOT NULL,
  created_row_ids JSON NOT NULL,
  expired_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY registration_expiries_log_unique (placement_log_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

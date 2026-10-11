CREATE TABLE IF NOT EXISTS withdrawal_submission_outbox (
  authorization_id CHAR(64) NOT NULL PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  security_version BIGINT UNSIGNED NOT NULL,
  terms JSON NOT NULL,
  terms_digest CHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'pending',
  remote_withdrawal_id BIGINT UNSIGNED NULL,
  attempts INT UNSIGNED NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  KEY withdrawal_submission_pending (status,updated_at),
  KEY withdrawal_submission_owner (user_id,created_at),
  UNIQUE KEY withdrawal_submission_remote (remote_withdrawal_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Source withdrawal authenticator replay guard. No secrets or codes stored.
CREATE TABLE IF NOT EXISTS withdrawal_totp_counters (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  last_counter BIGINT NOT NULL DEFAULT -1,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

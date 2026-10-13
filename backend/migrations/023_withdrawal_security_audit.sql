CREATE TABLE IF NOT EXISTS withdrawal_security_audit (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  event VARCHAR(64) NOT NULL,
  reference_id CHAR(64) NOT NULL,
  security_version BIGINT UNSIGNED NOT NULL,
  created_at TIMESTAMP NOT NULL,
  UNIQUE KEY withdrawal_security_audit_event (user_id,event,reference_id),
  KEY withdrawal_security_audit_owner (user_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

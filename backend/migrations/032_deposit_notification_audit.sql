CREATE TABLE IF NOT EXISTS deposit_notification_scans (
 user_id BIGINT UNSIGNED PRIMARY KEY,
 before_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
 last_checked_at DATETIME NOT NULL,
 KEY deposit_scan_rotation (last_checked_at, user_id)
) ENGINE=InnoDB;
CREATE TABLE IF NOT EXISTS deposit_notification_audit (
 remote_deposit_id BIGINT UNSIGNED PRIMARY KEY,
 user_id BIGINT UNSIGNED NOT NULL,
 amount VARCHAR(80) NOT NULL,
 tx_hash VARCHAR(66) NOT NULL,
 log_index BIGINT UNSIGNED NOT NULL,
 status VARCHAR(24) NOT NULL DEFAULT 'pending',
 created_at DATETIME NOT NULL,
 attempted_at DATETIME NULL,
 sent_at DATETIME NULL,
 UNIQUE KEY deposit_notification_event (tx_hash, log_index),
 KEY deposit_mail_queue (status, created_at, remote_deposit_id)
) ENGINE=InnoDB;

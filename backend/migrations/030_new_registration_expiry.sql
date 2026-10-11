-- Only registrations completed AFTER deployment enroll themselves. Never
-- backfill existing users: old pending accounts must remain untouched.
CREATE TABLE IF NOT EXISTS registration_expiry_enrollments (
  user_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
  policy_version VARCHAR(32) NOT NULL,
  registered_at TIMESTAMP NOT NULL,
  last_checked_at TIMESTAMP NULL,
  KEY expiry_enrollment_policy (policy_version,user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Independent tombstones have no user FK: hard deletion must not erase audit
-- evidence or the source identity used for custody/late-deposit recovery.
CREATE TABLE IF NOT EXISTS registration_account_deletions (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT UNSIGNED NOT NULL,
  placement_log_id BIGINT UNSIGNED NULL,
  reason VARCHAR(64) NOT NULL,
  deadline DATETIME NOT NULL,
  custody_identity VARCHAR(64) NOT NULL,
  account_snapshot JSON NOT NULL,
  package_snapshot JSON NOT NULL,
  purchase_snapshot JSON NOT NULL,
  placement_snapshot JSON NOT NULL,
  custody_seal JSON NULL,
  deleted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY account_deletion_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

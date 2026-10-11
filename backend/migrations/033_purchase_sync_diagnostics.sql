ALTER TABLE package_purchase_intents
 ADD COLUMN last_attempt_at DATETIME NULL,
 ADD COLUMN last_sync_error VARCHAR(64) NULL;

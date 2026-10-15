-- New purchases bind to the selected catalog ID, not a mutable user_packages row.
-- Historical rows are deliberately not guessed/backfilled across environments.
ALTER TABLE package_purchase_intents ADD COLUMN package_id BIGINT UNSIGNED NULL;

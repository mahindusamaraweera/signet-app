-- Preserve absence without inventing a mining ID or row.
ALTER TABLE historical_mining_cutovers MODIFY mining_id BIGINT UNSIGNED NULL;

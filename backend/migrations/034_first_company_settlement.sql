CREATE TABLE IF NOT EXISTS blockchain_company_settlements (
    purchase_intent_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    manifest_hash CHAR(64) NULL,
    last_checked_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_company_settlement_intent FOREIGN KEY (purchase_intent_id) REFERENCES package_purchase_intents(id)
);

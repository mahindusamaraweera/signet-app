CREATE TABLE blockchain_topup_commission_plans (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 purchase_intent_id BIGINT UNSIGNED NOT NULL,
 original_user_id BIGINT UNSIGNED NOT NULL,
 beneficiary_user_id BIGINT UNSIGNED NOT NULL,
 commission_kind VARCHAR(40) NOT NULL,
 commission_usd DECIMAL(36,18) NOT NULL,
 status VARCHAR(32) NOT NULL DEFAULT 'reserved',
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE KEY topup_commission_role (purchase_intent_id,commission_kind),
 KEY topup_commission_cap (beneficiary_user_id,status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE package_purchase_intents ADD COLUMN commission_plan_created BOOLEAN NOT NULL DEFAULT FALSE;

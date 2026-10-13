ALTER TABLE package_purchase_intents
 ADD COLUMN service_fee_usdt DECIMAL(36,18) NULL,
 ADD COLUMN required_usdt DECIMAL(36,18) NULL,
 ADD COLUMN bonus_percent BIGINT NULL;

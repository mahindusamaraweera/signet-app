ALTER TABLE password_reset ADD COLUMN delivery_status VARCHAR(32) NOT NULL DEFAULT 'legacy';

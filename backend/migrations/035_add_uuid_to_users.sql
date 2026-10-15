-- Adds users.uuid. Existing rows get a deterministic UUIDv7-shaped value
-- built from created_at (48-bit Unix ms, falling back to updated_at, then
-- the epoch) with the user id as the trailing 48 bits, so every value is
-- unique and re-running the derivation yields the same result. New users
-- get one from the application (registrationverification.newUUID); the
-- UUID() default covers any other writer to this shared table.
ALTER TABLE users ADD COLUMN uuid CHAR(36) NULL DEFAULT (UUID());

UPDATE users SET uuid = LOWER(CONCAT_WS('-',
    SUBSTRING(LPAD(HEX(FLOOR(UNIX_TIMESTAMP(COALESCE(created_at, updated_at, '1970-01-01 00:00:01')) * 1000)), 12, '0'), 1, 8),
    SUBSTRING(LPAD(HEX(FLOOR(UNIX_TIMESTAMP(COALESCE(created_at, updated_at, '1970-01-01 00:00:01')) * 1000)), 12, '0'), 9, 4),
    '7000',
    '8000',
    LPAD(HEX(id), 12, '0')));

ALTER TABLE users MODIFY COLUMN uuid CHAR(36) NOT NULL DEFAULT (UUID()), ADD UNIQUE KEY users_uuid_unique (uuid);

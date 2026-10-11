-- Serializes verified completion so legacy phone/binance uniqueness checks
-- are not raced by concurrent verified registrations.
CREATE TABLE IF NOT EXISTS registration_completion_locks (
  id TINYINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=InnoDB;
INSERT IGNORE INTO registration_completion_locks (id) VALUES (1);

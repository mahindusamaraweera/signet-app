-- New registrations stay here until email ownership is proven.
-- No user, wallet or placement is created before verification.
CREATE TABLE IF NOT EXISTS pending_email_registrations (
  id CHAR(64) NOT NULL PRIMARY KEY,
  email VARCHAR(255) NOT NULL,
  registration_data JSON NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  verification_hash CHAR(64) NOT NULL,
  status VARCHAR(20) NOT NULL DEFAULT 'pending_delivery',
  expires_at TIMESTAMP NOT NULL,
  verified_at TIMESTAMP NULL,
  user_id BIGINT UNSIGNED NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  KEY pending_registration_email (email, created_at),
  KEY pending_registration_expiry (status, expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Serializes pending signup/resend requests for one email across API replicas.
CREATE TABLE IF NOT EXISTS registration_email_guards (
  email VARCHAR(255) NOT NULL PRIMARY KEY,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

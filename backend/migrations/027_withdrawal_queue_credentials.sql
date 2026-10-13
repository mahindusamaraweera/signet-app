-- Old queued records intentionally remain NULL and cannot be signed.
-- Reauthorize rather than guessing which credentials approved them.
ALTER TABLE withdrawal_submission_outbox
  ADD COLUMN credential_revision CHAR(64) NULL;

ALTER TABLE blockchain_commission_outbox
  ADD COLUMN commission_kind VARCHAR(40) NOT NULL DEFAULT 'unclassified',
  ADD COLUMN recipient_address VARCHAR(42) NULL,
  ADD COLUMN quote_snapshot JSON NULL,
  ADD COLUMN last_checked_at TIMESTAMP NULL;

-- Do not infer roles from old descriptions or already-submitted instructions.
UPDATE blockchain_commission_outbox SET status='requires_reconciliation'
WHERE commission_kind='unclassified' AND status='awaiting_policy'
  AND remote_payment_id IS NULL AND transfer_tx_hash IS NULL;

-- Reclassify only untouched draft accounting rows; never alter a submitted payment.
UPDATE blockchain_commission_outbox
SET status='requires_reconciliation'
WHERE beneficiary_user_id=1
  AND description <> 'Income cap commission diversion'
  AND status='awaiting_policy'
  AND remote_payment_id IS NULL
  AND transfer_tx_hash IS NULL;

// Display only: the cancellation endpoint still enforces all financial guards.
export function canCancelRequest(request) {
  if (['cancel_requested', 'cancelled', 'cancellation_review_required'].includes(request.queue_status)) return false
  if (request.remote_withdrawal_id) {
    const payout = request.payout
    return !!payout && !payout.tx_hash && ['otp_pending', 'audit_pending', 'security_hold', 'audit_failed', 'approval_pending', 'queued', 'broadcast_failed'].includes(payout.status)
  }
  return ['pending', 'authorized'].includes(request.authorization_status)
    || (request.authorization_status === 'consumed' && request.queue_status === 'pending')
}

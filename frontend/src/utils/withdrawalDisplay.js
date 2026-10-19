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

// Never describe an authorization as unpaid when a custody request exists.
export function withdrawalRequestNotice(request) {
  if (request.remote_withdrawal_id || request.queue_status) return ''
  if (request.authorization_status === 'expired') return 'Verification expired before submission. Start a new withdrawal and complete verification before the expiry time.'
  if (request.authorization_status === 'invalidated') return 'This approval is no longer valid. Check your withdrawal security settings, then start a new withdrawal.'
  return ''
}

// A new financial intent requires a known end state for the previous request.
export function hasOutstandingWithdrawal(requests) {
  return requests.some(request => {
    if (['pending', 'cancel_requested', 'cancellation_review_required'].includes(request.queue_status)) return true
    if (request.remote_withdrawal_id) return !['completed', 'cancelled'].includes(request.payout?.status)
    return request.queue_status === 'submitted'
  })
}

// Custody and queue evidence take precedence over verification-only state.
export function withdrawalStatusLabel(request) {
  if (request.queue_status === 'cancel_requested') return 'Cancellation requested'
  if (request.queue_status === 'cancellation_review_required') return 'Cancellation needs review'
  const labels = {
    completed: 'Completed', cancelled: 'Cancelled', approval_pending: 'Awaiting admin approval',
    audit_pending: 'Security checks', external_security_pending: 'Security checks',
    security_hold: 'Security hold', audit_failed: 'Needs review', broadcast_failed: 'Needs review',
    queued: 'Processing', broadcasting: 'Sending payment', confirming: 'Confirming payment',
    otp_pending: 'Verification required',
  }
  if (request.remote_withdrawal_id) {
    return request.payout ? labels[request.payout.status] || 'Processing' : 'Payout status unavailable'
  }
  if (request.queue_status === 'pending' || request.queue_status === 'submitted') return 'Submitted · awaiting custody'
  if (request.queue_status === 'cancelled') return 'Cancelled'
  return { pending: 'Verification incomplete', authorized: 'Verified · submit withdrawal',
    expired: 'Verification expired', invalidated: 'Verification invalidated' }[request.authorization_status] || 'Status unavailable'
}

export function withdrawalAmount(value) {
  const text = String(value ?? '')
  if (!/^\d+(\.\d+)?$/.test(text)) return '—'
  const [whole, fraction = ''] = text.split('.')
  const digits = fraction.replace(/0+$/, '')
  return whole.replace(/^0+(?=\d)/, '').replace(/\B(?=(\d{3})+(?!\d))/g, ',') + (digits ? '.' + digits : '')
}

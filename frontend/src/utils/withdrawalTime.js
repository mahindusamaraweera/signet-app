// API dates must carry a time zone; never guess for a naive database timestamp.
export function deadlineMillis(value) {
  if (typeof value !== 'string' || !/(?:Z|[+-]\d{2}:\d{2})$/i.test(value)) return null
  const result = Date.parse(value)
  return Number.isFinite(result) ? result : null
}
export function remainingSeconds(deadline, now) {
  const end = deadlineMillis(deadline)
  return end === null || !Number.isFinite(now) ? null : Math.max(0, Math.ceil((end - now) / 1000))
}
export function countdownText(seconds) {
  if (seconds === null) return 'Time unavailable'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor(seconds % 86400 / 3600)
  const minutes = Math.floor(seconds % 3600 / 60)
  const rest = seconds % 60
  return `${days ? `${days}d ` : ''}${String(hours).padStart(2, '0')}h ${String(minutes).padStart(2, '0')}m ${String(rest).padStart(2, '0')}s`
}
export function localDeadline(value) {
  const time = deadlineMillis(value)
  return time === null ? 'Check status to load the deadline.' : new Intl.DateTimeFormat(undefined, {
    day: 'numeric', month: 'short', hour: 'numeric', minute: '2-digit', second: '2-digit', timeZoneName: 'short',
  }).format(new Date(time))
}

export function withdrawalErrorMessage(data) {
  if (['withdrawal_security_wait', 'withdrawal_address_wait'].includes(data?.code) && deadlineMillis(data.available_at) !== null) {
    const subject = data.code === 'withdrawal_security_wait' ? 'Account security wait' : 'Address waiting period'
    return `${subject} ends ${localDeadline(data.available_at)} (your local time). Check the countdown before trying again.`
  }
  return data?.message || 'Response uncertain. Check request status before retrying.'
}

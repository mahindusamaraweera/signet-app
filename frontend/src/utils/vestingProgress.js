const SCALE = 10n ** 18n

function amount(value) {
  if (typeof value !== 'string' || !/^\d{1,18}(\.\d{1,18})?$/.test(value)) throw new Error('Invalid amount')
  const [whole, fraction = ''] = value.split('.')
  return BigInt(whole) * SCALE + BigInt(fraction.padEnd(18, '0'))
}

function display(value) {
  return `${value / SCALE}.${((value % SCALE) / (10n ** 12n)).toString().padStart(6, '0')}`
}

// Display-only interpolation. This never calculates a wallet credit or claim amount.
export function vestingProgress(schedule, elapsedSinceFetchMs) {
  try {
    const p = schedule.progress_estimate
    if (!p?.display_only || p.settlement_interval !== 'daily') return null
    if (!Number.isFinite(elapsedSinceFetchMs) || elapsedSinceFetchMs < 0 || elapsedSinceFetchMs > 300000) return null
    const baseline = amount(p.baseline_amount)
    const total = amount(schedule.total_amount)
    const released = amount(schedule.released_amount)
    if (baseline > total || released > total) return null
    if (schedule.status !== 'active') return display(released)
    const start = Date.parse(p.baseline_at)
    const end = Date.parse(schedule.ends_at)
    const cliff = schedule.cliff_ends_at ? Date.parse(schedule.cliff_ends_at) : start
    const now = Date.parse(p.evaluated_at) + Math.floor(elapsedSinceFetchMs)
    if (![start, end, cliff, now].every(Number.isSafeInteger) || end < start) return null
    let value = baseline
    if (now >= cliff && now >= start) {
      value = now >= end ? total : baseline + (total - baseline) * BigInt(now - start) / BigInt(end - start)
    }
    // Settled balances are authoritative, including after a changed schedule.
    if (value < released) value = released
    return display(value > total ? total : value)
  } catch { return null }
}

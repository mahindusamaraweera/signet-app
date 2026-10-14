const retryable = new Set(['awaiting_deposit', 'reserved', 'pending_sweep', 'pending_swap', 'pending_vesting', 'retrying', 'awaiting_commission_settlement'])

function units(value) {
  const text = String(value ?? '')
  if (!/^\d{1,18}(\.\d{1,18})?$/.test(text)) return null
  const [whole, fraction = ''] = text.split('.')
  return BigInt(whole) * 10n ** 18n + BigInt(fraction.padEnd(18, '0'))
}

// Display guidance only. The custody service checks and reserves the actual
// confirmed ledger balance under lock; browser amounts never authorize funding.
export function activationAction(purchase, wallet) {
  if (!purchase?.id || !retryable.has(purchase.status) || purchase.placement_required) return null
  if (purchase.status !== 'awaiting_deposit') return { label: 'Check activation', ready: false }
  const required = units(purchase.required_usdt || purchase.package_price)
  const available = units(wallet?.available)
  const ready = required !== null && required > 0n && available !== null && available >= required
  return { label: ready ? (purchase.purchase_kind === 'topup' ? 'Activate top-up' : 'Activate package') : 'Check balance and activate', ready }
}

export function hasUSDTShortfall(value) {
  const amount = units(value)
  return amount !== null && amount > 0n
}

// A historical sync error is not evidence of a current placement problem.
export function activationCheckMessage(purchase) {
  if (purchase?.placement_required === true) return 'Your placement needs review before this purchase can proceed.'
  if (purchase?.last_sync_error === 'placement_required' && purchase?.placement_required === false) return 'Your placement is available now. Retry activation below or wait for the next automatic check.'
  if (purchase?.last_sync_error) return 'The last activation check could not complete. Your funds remain recorded. Retry activation below or contact support if this persists.'
  return 'The activation worker has not recorded its first check yet.'
}

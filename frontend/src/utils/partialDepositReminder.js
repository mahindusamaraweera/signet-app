// Use the server's exact deficit, never a floating-point price calculation.
export function partialDepositReminder(purchase) {
  if (!purchase || purchase.status !== 'awaiting_deposit' || purchase.deposit_seen !== true) return null
  const remaining = String(purchase.remaining_usdt ?? '')
  if (!/^\d+(\.\d{1,18})?$/.test(remaining) || !/[1-9]/.test(remaining)) return null
  return {
    key: `${purchase.id}:${remaining}`,
    remaining,
    topup: purchase.purchase_kind === 'topup',
  }
}

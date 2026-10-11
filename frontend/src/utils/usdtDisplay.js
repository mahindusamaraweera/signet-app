// Display only. Never use formatted amounts to authorize or build a transfer.
export function usdtDisplay(value) {
  const text = String(value ?? '')
  if (!/^\d+(\.\d{1,18})?$/.test(text)) return '—'
  const [whole, fraction = ''] = text.split('.')
  let cents = BigInt(whole) * 100n + BigInt(fraction.padEnd(2, '0').slice(0, 2))
  if (Number(fraction[2] || '0') >= 5) cents += 1n
  return `${(cents / 100n).toString().replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${(cents % 100n).toString().padStart(2, '0')}`
}

export function usdtDeficitDisplay(value) {
  const text = String(value ?? '')
  if (/^0+(\.\d{1,18})?$/.test(text) && /[1-9]/.test(text) && BigInt((text.split('.')[1] || '').padEnd(18, '0')) < 10000000000000000n) return 'less than 0.01'
  return usdtDisplay(value)
}

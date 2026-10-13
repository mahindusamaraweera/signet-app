// Display only: keep monetary API strings unchanged for funding/authorization.
export function assetDisplay(value, asset = 'USDT') {
  const text = String(value ?? '')
  if (!/^\d+(\.\d+)?$/.test(text)) return '—'
  const precision = asset === 'USDT' ? 2 : 4
  const [whole, fraction = ''] = text.split('.')
  const integer = whole.replace(/^0+(?=\d)/, '')
  const shown = fraction.slice(0, precision).padEnd(precision, '0')
  if (integer === '0' && /^0+$/.test(shown) && /[1-9]/.test(fraction)) return `<0.${'0'.repeat(precision - 1)}1`
  return `${integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',')}.${shown}`
}

// Read-only reconciliation. Older source approvals may refer to payouts beyond
// custody's first page; do not mistake a missing first-page row for unpaid funds.
export async function loadWithdrawalPayouts(api, remoteIds) {
  const wanted = new Set(remoteIds.filter(Boolean).map(String)), payouts = new Map(), seen = new Set()
  if (!wanted.size) return payouts
  let before
  for (let page = 0; page < 10; page++) {
    const { data } = await api.get('/blockchain/withdrawals', before ? { params: { before_id: before } } : undefined)
    for (const payout of data.withdrawals || []) payouts.set(String(payout.id), payout)
    if ([...wanted].every(id => payouts.has(id))) break
    const next = data.next_before_id
    if (!next || seen.has(String(next))) break
    seen.add(String(next)); before = String(next)
  }
  return payouts
}

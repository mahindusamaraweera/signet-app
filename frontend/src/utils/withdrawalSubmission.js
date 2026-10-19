// Queue only after an explicit successful verification response. Never retry
// either financial-intent request automatically on an uncertain response.
export async function verifyAndQueueWithdrawal(api, body, codes, onVerified) {
  await api.post('/blockchain/withdrawals/confirm', { ...body, ...codes })
  if (!onVerified()) return false
  await api.post('/blockchain/withdrawals/queue', body)
  return true
}

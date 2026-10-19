// Queue only after an explicit successful verification response. Never retry
// either financial-intent request automatically on an uncertain response.
export async function verifyAndQueueWithdrawal(api, body, codes, onVerified) {
  await api.post('/blockchain/withdrawals/confirm', { ...body, ...codes })
  if (!onVerified()) return false
  await api.post('/blockchain/withdrawals/queue', body)
  return true
}

// Only explicit code rejections are safe to retry in the same unapproved flow.
// Unknown/network failures and queue failures still require reconciliation.
export function withdrawalCodeRetry(error, action, stage) {
  if (action !== 'confirm' || stage !== 'prepared' || error?.response?.status !== 409) return null
  const code = error.response.data?.code
  if (code === 'withdrawal_email_incorrect') return { field: 'email', message: 'Email code is incorrect. Use the newest withdrawal email and try again. Nothing was submitted.' }
  if (code === 'withdrawal_authenticator_invalid') return { field: 'authenticator', message: 'Authenticator code is incorrect or expired. Enter a fresh code and try again. Nothing was submitted.' }
  if (code === 'withdrawal_authenticator_reused') return { field: 'authenticator', message: 'Authenticator code was already used. Wait for the next code and try again. Nothing was submitted.' }
  return null
}

import test from 'node:test'
import assert from 'node:assert/strict'
import { loadWithdrawalPayouts } from '../src/utils/withdrawalHistory.js'
import { hasOutstandingWithdrawal } from '../src/utils/withdrawalDisplay.js'

test('reconciles an older completed payout beyond the first custody page', async () => {
  const calls = []
  const api = { get: async (path, options) => {
    calls.push(options?.params?.before_id)
    return { data: calls.length === 1
      ? { withdrawals: [{ id: '100', status: 'completed' }], next_before_id: '100' }
      : { withdrawals: [{ id: '50', status: 'completed' }], next_before_id: null } }
  } }
  const payouts = await loadWithdrawalPayouts(api, ['50'])
  assert.deepEqual(calls, [undefined, '100'])
  assert.equal(hasOutstandingWithdrawal([{ queue_status: 'submitted', remote_withdrawal_id: '50', payout: payouts.get('50') }]), false)
})
test('unknown or active payouts prevent starting another withdrawal', () => {
  for (const status of [undefined, 'queued', 'audit_failed', 'broadcast_failed']) {
    assert.equal(hasOutstandingWithdrawal([{ queue_status: 'submitted', remote_withdrawal_id: '50', payout: status ? { status } : null }]), true)
  }
  for (const status of ['pending', 'cancel_requested', 'cancellation_review_required']) assert.equal(hasOutstandingWithdrawal([{ queue_status: status }]), true)
  assert.equal(hasOutstandingWithdrawal([{ authorization_status: 'expired', queue_status: '' }]), false)
})
test('history paging stops at repeated cursors and propagates uncertain responses', async () => {
  let count = 0
  await loadWithdrawalPayouts({ get: async () => { count++; return { data: { withdrawals: [], next_before_id: '100' } } } }, ['50'])
  assert.equal(count, 2)
  await assert.rejects(loadWithdrawalPayouts({ get: async () => { throw new Error('unavailable') } }, ['50']))
})

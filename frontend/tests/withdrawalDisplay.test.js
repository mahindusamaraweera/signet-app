import test from 'node:test'
import assert from 'node:assert/strict'
import { canCancelRequest } from '../src/utils/withdrawalDisplay.js'

test('paid, expired and unknown remote payouts cannot offer cancellation', () => {
  assert.equal(canCancelRequest({ authorization_status: 'expired' }), false)
  const submitted = { authorization_status: 'consumed', queue_status: 'submitted', remote_withdrawal_id: '508' }
  assert.equal(canCancelRequest(submitted), false)
  assert.equal(canCancelRequest({ ...submitted, payout: { status: 'completed', tx_hash: '0x123' } }), false)
  assert.equal(canCancelRequest({ ...submitted, payout: { status: 'queued', tx_hash: '0x123' } }), false)
  assert.equal(canCancelRequest({ ...submitted, payout: { status: 'approval_pending', tx_hash: null } }), true)
  assert.equal(canCancelRequest({ ...submitted, queue_status: 'cancel_requested', payout: { status: 'queued' } }), false)
})

test('live approvals and queued requests retain cancellation', () => {
  assert.equal(canCancelRequest({ authorization_status: 'authorized' }), true)
  assert.equal(canCancelRequest({ authorization_status: 'consumed', queue_status: 'pending' }), true)
})

test('expired approval guidance never overrides a queued or remote payment', async () => {
  const { withdrawalRequestNotice } = await import('../src/utils/withdrawalDisplay.js')
  assert.match(withdrawalRequestNotice({ authorization_status: 'expired' }), /Start a new withdrawal/)
  assert.match(withdrawalRequestNotice({ authorization_status: 'invalidated' }), /security settings/)
  assert.equal(withdrawalRequestNotice({ authorization_status: 'expired', queue_status: 'pending' }), '')
  assert.equal(withdrawalRequestNotice({ authorization_status: 'expired', remote_withdrawal_id: '508' }), '')
})

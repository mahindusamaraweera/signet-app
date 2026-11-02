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

test('status labels distinguish verification from custody processing', async () => {
  const { withdrawalStatusLabel } = await import('../src/utils/withdrawalDisplay.js')
  assert.equal(withdrawalStatusLabel({ authorization_status: 'pending' }), 'Verification incomplete')
  assert.equal(withdrawalStatusLabel({ authorization_status: 'expired' }), 'Verification expired')
  assert.equal(withdrawalStatusLabel({ authorization_status: 'expired', queue_status: 'pending' }), 'Submitted · awaiting custody')
  assert.equal(withdrawalStatusLabel({ authorization_status: 'expired', remote_withdrawal_id: '5', payout: { status: 'completed' } }), 'Completed')
  assert.equal(withdrawalStatusLabel({ remote_withdrawal_id: '5' }), 'Payout status unavailable')
  assert.equal(withdrawalStatusLabel({ remote_withdrawal_id: '5', payout: { status: 'approval_pending' } }), 'Awaiting admin approval')
})

test('withdrawal amounts remove padding while retaining every significant digit', async () => {
  const { withdrawalAmount } = await import('../src/utils/withdrawalDisplay.js')
  assert.equal(withdrawalAmount('22588.000000000000000000'), '22,588')
  assert.equal(withdrawalAmount('677.640000000000000000'), '677.64')
  assert.equal(withdrawalAmount('999999999999999999.000000000000000001'), '999,999,999,999,999,999.000000000000000001')
  assert.equal(withdrawalAmount(null), '—')
})

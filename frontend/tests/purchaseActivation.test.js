import test from 'node:test'
import assert from 'node:assert/strict'
import { activationAction } from '../src/utils/purchaseActivation.js'

const purchase = { id: 7, status: 'awaiting_deposit', package_price: '1', required_usdt: '1.10', purchase_kind: 'first' }
test('activation accounts for the service fee and exact base units', () => {
  assert.equal(activationAction(purchase, { available: '1' }).ready, false)
  assert.equal(activationAction(purchase, { available: '1.099999999999999999' }).ready, false)
  assert.equal(activationAction(purchase, { available: '1.10' }).label, 'Activate package')
  assert.equal(activationAction(purchase, { available: '2' }).ready, true)
})
test('reserved funds do not demand a second deposit', () => {
  for (const status of ['reserved', 'pending_sweep', 'pending_swap', 'pending_vesting', 'retrying', 'awaiting_commission_settlement']) {
    assert.equal(activationAction({ ...purchase, status }, { available: '0' }).label, 'Check activation')
  }
})
test('completed, expired, missing, and blocked purchases have no activation button', () => {
  for (const status of ['completed', 'expired', 'awaiting_reopening', 'awaiting_cap_review', 'unknown']) assert.equal(activationAction({ ...purchase, status }, { available: '10' }), null)
  assert.equal(activationAction({ ...purchase, placement_required: true }, { available: '10' }), null)
  assert.equal(activationAction(null, { available: '10' }), null)
})
test('top-up and stale/malformed balance guidance stays explicit', () => {
  assert.equal(activationAction({ ...purchase, purchase_kind: 'topup' }, { available: '1.10' }).label, 'Activate top-up')
  for (const available of [undefined, '-1', 'NaN', '1e3', '1.0000000000000000001']) assert.equal(activationAction(purchase, { available }).ready, false)
})

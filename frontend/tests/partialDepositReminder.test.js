import test from 'node:test'
import assert from 'node:assert/strict'
import { partialDepositReminder } from '../src/utils/partialDepositReminder.js'

test('partial deposits show the exact server deficit and preserve account messaging', () => {
  const notice = partialDepositReminder({ id: 42, status: 'awaiting_deposit', deposit_seen: true, remaining_usdt: '1030.000000000000000001' })
  assert.equal(notice.remaining, '1030.000000000000000001')
  assert.equal(notice.key, '42:1030.000000000000000001')
  assert.equal(notice.topup, false)
})
test('one base unit short still triggers a reminder', () => {
  assert.ok(partialDepositReminder({ status: 'awaiting_deposit', deposit_seen: true, remaining_usdt: '0.000000000000000001' }))
})
test('zero, excess, unpaid, processing and malformed deficits never trigger', () => {
  for (const remaining of ['0', '0.000000000000000000', '-1', 'NaN', '', '1e3']) {
    assert.equal(partialDepositReminder({ status: 'awaiting_deposit', deposit_seen: true, remaining_usdt: remaining }), null)
  }
  assert.equal(partialDepositReminder({ status: 'awaiting_deposit', deposit_seen: false, remaining_usdt: '1130' }), null)
  for (const status of ['swapping', 'completed', 'expired']) {
    assert.equal(partialDepositReminder({ status, deposit_seen: true, remaining_usdt: '1' }), null)
  }
})
test('top-up reminder identifies the top-up without offering new vesting', () => {
  assert.equal(partialDepositReminder({ status: 'awaiting_deposit', deposit_seen: true, remaining_usdt: '10', purchase_kind: 'topup' }).topup, true)
})

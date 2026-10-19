import test from 'node:test'
import assert from 'node:assert/strict'
import { verifyAndQueueWithdrawal } from '../src/utils/withdrawalSubmission.js'

test('verification queues the same approved identity without sending codes to queue', async () => {
  const calls = [], body = { id: 'same-id', proof: 'same-proof', password: 'sample', terms: { gross: '100' } }
  const api = { post: async (path, data) => calls.push([path, data]) }
  assert.equal(await verifyAndQueueWithdrawal(api, body, { email_code: '123456', authenticator_code: '654321' }, () => true), true)
  assert.equal(calls.length, 2)
  assert.equal(calls[0][0], '/blockchain/withdrawals/confirm')
  assert.equal(calls[1][0], '/blockchain/withdrawals/queue')
  assert.deepEqual(calls[1][1], body)
})
test('failed or uncertain verification never queues and uncertain queue is never retried', async () => {
  for (const failAt of ['confirm', 'queue']) {
    const calls = []
    const api = { post: async path => { calls.push(path); if (path.endsWith(failAt)) throw new Error('uncertain') } }
    await assert.rejects(verifyAndQueueWithdrawal(api, {}, {}, () => true))
    assert.equal(calls.length, failAt === 'confirm' ? 1 : 2)
  }
})
test('leaving the page after verification prevents queue submission', async () => {
  const calls = []
  assert.equal(await verifyAndQueueWithdrawal({ post: async p => calls.push(p) }, {}, {}, () => false), false)
  assert.equal(calls.length, 1)
})

test('only explicit code failures allow retry before approval', async () => {
  const { withdrawalCodeRetry } = await import('../src/utils/withdrawalSubmission.js')
  for (const [code, field] of [['withdrawal_email_incorrect', 'email'], ['withdrawal_authenticator_invalid', 'authenticator'], ['withdrawal_authenticator_reused', 'authenticator']]) {
    const error = { response: { status: 409, data: { code } } }
    assert.equal(withdrawalCodeRetry(error, 'confirm', 'prepared').field, field)
    assert.equal(withdrawalCodeRetry(error, 'confirm', 'authorized'), null)
    assert.equal(withdrawalCodeRetry(error, 'queue', 'prepared'), null)
    const calls = []
    await assert.rejects(verifyAndQueueWithdrawal({ post: async path => { calls.push(path); throw error } }, {}, {}, () => true))
    assert.deepEqual(calls, ['/blockchain/withdrawals/confirm'])
  }
  assert.equal(withdrawalCodeRetry(new Error('network'), 'confirm', 'prepared'), null)
  assert.equal(withdrawalCodeRetry({ response: { status: 409, data: { code: 'withdrawal_verification_failed' } } }, 'confirm', 'prepared'), null)
})

test('a corrected code retries the same approval and queues exactly once', async () => {
  const calls = [], body = { id: 'original-approval', proof: 'original-proof', terms: { gross: '78000' } }
  let rejectCode = true
  const api = { post: async (path, data) => {
    calls.push([path, data])
    if (path.endsWith('/confirm') && rejectCode) {
      rejectCode = false
      throw { response: { status: 409, data: { code: 'withdrawal_authenticator_invalid' } } }
    }
  } }
  await assert.rejects(verifyAndQueueWithdrawal(api, body, { email_code: '123456', authenticator_code: '000000' }, () => true))
  assert.equal(calls.length, 1)
  await verifyAndQueueWithdrawal(api, body, { email_code: '123456', authenticator_code: '654321' }, () => true)
  assert.deepEqual(calls.map(([path]) => path), ['/blockchain/withdrawals/confirm', '/blockchain/withdrawals/confirm', '/blockchain/withdrawals/queue'])
  assert.equal(calls.every(([, data]) => data.id === body.id), true)
  assert.deepEqual(calls[2][1], body)
})

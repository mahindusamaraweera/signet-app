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

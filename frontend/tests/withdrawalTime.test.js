import test from 'node:test'
import assert from 'node:assert/strict'
import { deadlineMillis, remainingSeconds, countdownText, localDeadline } from '../src/utils/withdrawalTime.js'

test('offset dates represent the same instant and countdown rounds up', () => {
  const utc = '2026-10-19T14:16:49Z', colombo = '2026-10-19T19:46:49+05:30'
  assert.equal(deadlineMillis(utc), deadlineMillis(colombo))
  assert.equal(remainingSeconds(utc, deadlineMillis(utc) - 500), 1)
  assert.equal(remainingSeconds(utc, deadlineMillis(utc)), 0)
  assert.equal(remainingSeconds(utc, deadlineMillis(utc) + 1000), 0)
  assert.equal(countdownText(90061), '1d 01h 01m 01s')
})
test('invalid and timezone-free deadlines cannot appear eligible', () => {
  for (const value of [null, '', 'bad', '2026-10-19 19:46:49']) {
    assert.equal(remainingSeconds(value, Date.now()), null)
  }
})
test('local display converts UTC to the viewer timezone', () => {
  const original = process.env.TZ
  try {
    process.env.TZ = 'Asia/Colombo'
    const lk = localDeadline('2026-10-19T14:16:49Z')
    process.env.TZ = 'Asia/Dubai'
    const dubai = localDeadline('2026-10-19T14:16:49Z')
    assert.notEqual(lk, dubai)
    assert.ok(!lk.includes('T14:16:49Z'))
  } finally { if (original) process.env.TZ = original; else delete process.env.TZ }
})

test('wait errors show local deadlines while other exact errors are preserved', async () => {
  const { withdrawalErrorMessage } = await import('../src/utils/withdrawalTime.js')
  const message = withdrawalErrorMessage({ code: 'withdrawal_security_wait', available_at: '2026-10-19T14:16:49Z', message: 'raw UTC message' })
  assert.match(message, /your local time/)
  assert.ok(!message.includes('raw UTC'))
  assert.equal(withdrawalErrorMessage({ code: 'withdrawal_email_incorrect', message: 'The email code is incorrect.' }), 'The email code is incorrect.')
})


test('shared token cooldown errors display a local deadline', () => {
  const data = { code: 'withdrawal_cooldown_active', available_at: '2026-10-19T14:16:49Z' }
  // The same formatter is used for account, address and custody cooldown waits.
  return import('../src/utils/withdrawalTime.js').then(({withdrawalErrorMessage}) => {
    assert.match(withdrawalErrorMessage(data), /Withdrawal cooldown ends/)
    assert.match(withdrawalErrorMessage(data), /your local time/)
  })
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { pastedAuthenticatorCode } from '../src/utils/authenticatorCode.js'

test('grouped clipboard codes normalize before input length enforcement', () => {
  assert.equal(pastedAuthenticatorCode('123 456'), '123456')
  assert.equal(pastedAuthenticatorCode(' 005\u00a0924\n'), '005924')
  assert.equal(pastedAuthenticatorCode('081804'), '081804')
  assert.equal(pastedAuthenticatorCode('1234567'), '123456')
  assert.equal(pastedAuthenticatorCode(null), '')
})

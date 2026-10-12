import test from 'node:test'
import assert from 'node:assert/strict'
import { assetDisplay } from '../src/utils/assetDisplay.js'

test('large ledger values keep precision and readable grouping', () => {
  assert.equal(assetDisplay('123456789012345678.123456789012345678'), '123,456,789,012,345,678.12')
  assert.equal(assetDisplay('1245.5123456789', 'SIGNET'), '1,245.5123')
})
test('small positive balances never appear as zero; missing values are unavailable', () => {
  assert.equal(assetDisplay('0.000000000000000001'), '<0.01')
  assert.equal(assetDisplay('0.000000000000000001', 'SIGNET'), '<0.0001')
  assert.equal(assetDisplay('0'), '0.00')
  assert.equal(assetDisplay(null), '—')
  assert.equal(assetDisplay('invalid'), '—')
})

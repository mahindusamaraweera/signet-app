import test from 'node:test'
import assert from 'node:assert/strict'
import { usdtDisplay, usdtDeficitDisplay } from '../src/utils/usdtDisplay.js'
test('USDT display uses two decimals without floating precision loss', () => {
  assert.equal(usdtDisplay('1000.000000000000000000'), '1,000.00')
  assert.equal(usdtDisplay('0.10'), '0.10')
  assert.equal(usdtDisplay('1.10'), '1.10')
  assert.equal(usdtDisplay('999.995'), '1,000.00')
  assert.equal(usdtDisplay('9007199254740993.12'), '9,007,199,254,740,993.12')
  assert.equal(usdtDisplay(null), '—')
  assert.equal(usdtDisplay('bad'), '—')
})
test('positive sub-cent deficits are not displayed as zero', () => {
  assert.equal(usdtDeficitDisplay('0.000000000000000001'), 'less than 0.01')
  assert.equal(usdtDeficitDisplay('0'), '0.00')
  assert.equal(usdtDeficitDisplay('0.01'), '0.01')
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { visiblePackageChoices } from '../src/utils/packageDisplay.js'

const packages = [500, 2500, 100, 1000, 5000].map(price => ({ id: String(price), price }))
test('defaults to main packages starting at 1000 without mutating catalog', () => {
  assert.deepEqual(visiblePackageChoices(packages, true, false).map(p => p.price), [1000, 2500, 5000])
  assert.deepEqual(packages.map(p => p.price), [500, 2500, 100, 1000, 5000])
})
test('optional lower packages appear after main packages', () => {
  assert.deepEqual(visiblePackageChoices(packages, true, true).map(p => p.price), [1000, 2500, 5000, 100, 500])
})
test('non-blockchain selection retains ascending price behavior', () => {
  assert.deepEqual(visiblePackageChoices(packages, false, false).map(p => p.price), [100, 500, 1000, 2500, 5000])
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { vestingProgress } from '../src/utils/vestingProgress.js'

const start = Date.parse('2026-10-10T00:00:00Z')
const schedule = {
  status: 'active', total_amount: '1000', released_amount: '200',
  cliff_ends_at: new Date(start).toISOString(),
  ends_at: new Date(start + 10000 * 86400000).toISOString(),
  progress_estimate: { baseline_amount: '200', baseline_at: new Date(start).toISOString(),
    evaluated_at: new Date(start).toISOString(), display_only: true, settlement_interval: 'daily' },
}

test('interpolates each second without losing released baseline', () => {
  assert.equal(vestingProgress(schedule, 0), '200.000000')
  assert.equal(vestingProgress(schedule, 1000), '200.000000')
  assert.equal(vestingProgress(schedule, 2000), '200.000001')
  assert.equal(vestingProgress({ ...schedule, progress_estimate: { ...schedule.progress_estimate,
    evaluated_at: new Date(start + 86400000).toISOString() } }, 0), '200.080000')
})

test('lock prevents growth and total bounds final estimate', () => {
  assert.equal(vestingProgress({ ...schedule, cliff_ends_at: new Date(start + 2000 * 86400000).toISOString() }, 2000), '200.000000')
  assert.equal(vestingProgress({ ...schedule, progress_estimate: { ...schedule.progress_estimate,
    evaluated_at: schedule.ends_at } }, 1000), '1000.000000')
})

test('stale, malformed and non-display-only snapshots are not estimated', () => {
  assert.equal(vestingProgress(schedule, 300001), null)
  assert.equal(vestingProgress({ ...schedule, total_amount: 'not-money' }, 0), null)
  assert.equal(vestingProgress({ ...schedule, progress_estimate: null }, 0), null)
  assert.equal(vestingProgress({ ...schedule, progress_estimate: { ...schedule.progress_estimate, display_only: false } }, 0), null)
})

test('suspended schedules freeze at settled amount and large amounts retain precision', () => {
  assert.equal(vestingProgress({ ...schedule, status: 'cancelled' }, 2000), '200.000000')
  assert.equal(vestingProgress({ ...schedule, total_amount: '999999999999999999.999999999999999999',
    released_amount: '999999999999999999.999999999999999999', status: 'completed' }, 0), '999999999999999999.999999')
})

test('option 2 waits 2000 days then interpolates over 7500 days, not 10000', () => {
  const releaseStart = start + 2000 * 86400000
  const end = releaseStart + 7500 * 86400000
  const option2 = {
    ...schedule, total_amount: '7500', released_amount: '0',
    starts_at: new Date(releaseStart).toISOString(),
    cliff_ends_at: new Date(releaseStart).toISOString(), ends_at: new Date(end).toISOString(),
    progress_estimate: { ...schedule.progress_estimate, baseline_amount: '0',
      baseline_at: new Date(releaseStart).toISOString(), evaluated_at: new Date(releaseStart - 1000).toISOString() },
  }
  assert.equal(vestingProgress(option2, 0), '0.000000')
  assert.equal(vestingProgress(option2, 1000), '0.000000')
  assert.equal(vestingProgress(option2, 2000), '0.000011')
  assert.equal(vestingProgress({ ...option2, progress_estimate: { ...option2.progress_estimate,
    evaluated_at: new Date(releaseStart + 86400000).toISOString() } }, 0), '1.000000')
  assert.equal(vestingProgress({ ...option2, progress_estimate: { ...option2.progress_estimate,
    evaluated_at: new Date(end).toISOString() } }, 0), '7500.000000')
})

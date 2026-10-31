import test from 'node:test'
import assert from 'node:assert/strict'
import { prometheusSeriesPoints } from '../src/utils/prometheusSeries.mjs'

test('missing evaluations break lines without changing returned values', () => {
  assert.deepEqual(prometheusSeriesPoints([[0, '1'], [15, '0'], [60, '4'], [75, '5']], 15),
    [[0, 1], [15000, 0], [30000, null], [60000, 4], [75000, 5]])
})

test('continuous fractional-step samples remain connected', () => {
  assert.deepEqual(prometheusSeriesPoints([[1.1, '2'], [1.2, '3'], [1.3, '4']], 0.1),
    [[1100, 2], [1200, 3], [1300, 4]])
})

test('non-finite and missing values become gaps rather than zero', () => {
  assert.deepEqual(prometheusSeriesPoints([[0, 'NaN'], [1, '+Inf'], [2, '-Inf'], [3, null], [4, '0']], 1),
    [[0, null], [1000, null], [2000, null], [3000, null], [4000, 0]])
})

test('empty and instant results do not invent samples', () => {
  assert.deepEqual(prometheusSeriesPoints(undefined, 15), [])
  assert.deepEqual(prometheusSeriesPoints([[10, '3']], null), [[10000, 3]])
})

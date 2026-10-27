import test from 'node:test'
import assert from 'node:assert/strict'
import { dashboardKey, moveDashboard } from '../src/utils/dashboardOrder.js'

test('cards move forward and backward without losing items', () => {
  const items = [{kind:'grafana',id:1},{kind:'cloud',id:1},{kind:'logs',id:1}]
  assert.deepEqual(moveDashboard(items, 'grafana:1', 2).map(dashboardKey), ['cloud:1','logs:1','grafana:1'])
  assert.deepEqual(moveDashboard(items, 'logs:1', 0).map(dashboardKey), ['logs:1','grafana:1','cloud:1'])
  assert.deepEqual(moveDashboard(items, 'logs:1', -1), items)
  assert.deepEqual(moveDashboard(items, 'missing:1', 0), items)
})

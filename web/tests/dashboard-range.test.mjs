import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'
import { computed, ref } from 'vue'

function dashboard() {
  const pending = []
  const source = readFileSync(new URL('../src/views/Dashboard.vue', import.meta.url), 'utf8')
    .split('<script setup>')[1].split('</script>')[0]
    .replace(/^import .*\n/gm, '').replace(/^use\(\[.*\]\)\n/m, '')
  const context = {
    computed, ref, onMounted() {}, onBeforeUnmount() {},
    useUserStore: () => ({ hasPermission: () => true }),
    useThemeColors: () => ref({}),
    getDashboardOverview: range => new Promise(resolve => pending.push({ range, resolve })),
  }
  for (const name of ['Refresh', 'Bell', 'Tickets', 'Cpu', 'Aim', 'Warning', 'FolderOpened', 'Document']) context[name] = name
  const state = vm.runInNewContext(source + '\n;({range, loadOverview, alertOption, notificationOption, probeOption, automationOption, periodMetrics, activeTrend, selectedTrend, offlineAgents, selectTrendWithKey})', context)
  return { state, pending }
}
const response = (range, time, count) => ({ data: { range, summary: {}, points: [{ time, alerts: count, notifications_success: count, notifications_failed: 0, probe_success: count, probe_failed: 0, automation_success: count, automation_failed: 0, automation_running: 9 }] } })

test('all charts follow the same date response and stale requests cannot overwrite it', async () => {
  const { state, pending } = dashboard()
  const today = state.loadOverview()
  pending[0].resolve(response('today', '2026-11-01 08:00', 2))
  await today
  state.range.value = '7d'
  const week = state.loadOverview()
  assert.equal(state.alertOption.value.xAxis.data[0], '08:00', 'old data retains its original time labels while loading')
  state.range.value = '30d'
  const month = state.loadOverview()
  pending[2].resolve(response('30d', '2026-11-01', 8))
  await month
  pending[1].resolve(response('7d', '2026-10-31', 3))
  await week
  for (const option of [state.alertOption, state.notificationOption, state.probeOption, state.automationOption]) {
    assert.equal(option.value.xAxis.data[0], '11-01')
    assert.equal(option.value.series[0].data[0], 8)
  }
  assert.equal(state.periodMetrics.value[0].value, 8)
  assert.equal(state.periodMetrics.value[3].value, '100.0%', 'running executions do not dilute completed success rate')
})

test('no completed executions means no success percentage', async () => {
  const { state, pending } = dashboard()
  const load = state.loadOverview()
  pending[0].resolve(response('today', '2026-11-01 08:00', 0))
  await load
  for (const metric of state.periodMetrics.value.slice(1)) assert.equal(metric.value, '—')
})


test('trend selection changes the displayed series and keyboard navigation follows the selected tab', async () => {
  const { state, pending } = dashboard()
  const load = state.loadOverview()
  pending[0].resolve(response('today', '2026-11-01 08:00', 4))
  await load
  assert.equal(state.selectedTrend.value.label, '告警发生')
  state.activeTrend.value = 'notifications'
  assert.equal(state.selectedTrend.value.option.series[0].name, '发送成功')
  assert.equal(state.selectedTrend.value.count, 4)
  let focused = -1
  const children = Array.from({ length: 4 }, (_, index) => ({ focus: () => { focused = index } }))
  state.selectTrendWithKey({ key: 'End', preventDefault() {}, currentTarget: { parentElement: { children } } }, 1)
  assert.equal(focused, 3)
  assert.equal(state.activeTrend.value, 'automation')
  assert.equal(state.selectedTrend.value.option.series.length, 3)
  state.selectTrendWithKey({ key: 'ArrowRight', preventDefault() {}, currentTarget: { parentElement: { children } } }, 3)
  assert.equal(state.activeTrend.value, 'alerts')
  assert.equal(focused, 0)
})

test('offline probe count distinguishes missing data from zero and derives from current inventory', async () => {
  const { state, pending } = dashboard()
  assert.equal(state.offlineAgents.value, null)
  const load = state.loadOverview()
  const result = response('today', '2026-11-01 08:00', 0)
  result.data.summary = { probe_agents: 8, probe_agents_online: 5 }
  pending[0].resolve(result)
  await load
  assert.equal(state.offlineAgents.value, 3)
})

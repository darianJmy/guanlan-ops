<template>
  <div class="dashboard">
    <el-empty v-if="!canView" description="暂无查看运维概览的权限，请联系管理员授权" />
    <template v-else>
      <header class="workspace-heading">
        <div class="workspace-intro">
          <div class="workspace-badge"><span></span>统一观测 · 智能运维</div>
          <p>洞察运行全貌，让异常发现更及时、故障处置更高效。</p>
          <div class="workspace-tags"><span><el-icon><Bell /></el-icon>告警感知</span><span><el-icon><Aim /></el-icon>主动探测</span><span><el-icon><Cpu /></el-icon>自动化响应</span></div>
        </div>
        <div class="orbital-art" aria-hidden="true">
          <div class="orbit-halo"></div><div class="orbit-floor"></div>
          <svg class="signal-trails" viewBox="0 0 440 280" fill="none">
            <path d="M0 178h70l40-42h96M440 86h-74l-52 50h-77M42 40h62l48 49h52M439 230h-85l-61-58h-56" />
            <path class="signal-highlight" d="M0 178h70l40-42h96M440 86h-74l-52 50h-77" />
          </svg>
          <i class="orbit orbit-one"></i><i class="orbit orbit-two"></i><i class="orbit orbit-three"></i>
          <div class="orbit-core">
            <div class="core-inner">
              <svg class="core-robot" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M12 8V4H8" />
                <rect width="16" height="12" x="4" y="8" rx="2" />
                <path d="M2 14h2m16 0h2m-7-1v2m-6-2v2" />
              </svg>
            </div>
          </div>
          <span class="orbit-node node-one"><el-icon><Bell /></el-icon><small>告警</small></span>
          <span class="orbit-node node-two"><el-icon><Aim /></el-icon><small>探测</small></span>
          <span class="orbit-node node-three"><el-icon><Tickets /></el-icon><small>执行</small></span>
        </div>
      </header>
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />

      <section class="watch-summary" aria-label="当前需要关注的状态" :aria-busy="loading">
        <div class="alert-focus" :class="{ 'has-alerts': summary.active_alerts > 0 }">
          <div class="focus-label"><el-icon><Bell /></el-icon>当前活跃告警</div>
          <div class="focus-value"><strong>{{ summary.active_alerts ?? '—' }}</strong><span>条</span></div>
          <p>{{ !overview ? '等待获取当前告警状态' : summary.active_alerts > 0 ? '存在活跃告警，请关注影响范围与处置进度。' : '当前暂无活跃告警，持续关注运行变化。' }}</p>
        </div>
        <div class="watch-item" :class="{ 'is-warning': summary.probe_tasks_unhealthy > 0 }">
          <div class="watch-label"><el-icon><Aim /></el-icon><h2>异常拨测</h2></div>
          <div class="watch-value"><strong>{{ summary.probe_tasks_unhealthy ?? '—' }}</strong><span>个任务</span></div>
          <p>{{ !overview ? '等待统计' : `共 ${summary.probe_tasks ?? 0} 个任务，已启用 ${summary.probe_tasks_enabled ?? 0} 个` }}</p>
        </div>
        <div class="watch-item" :class="{ 'is-warning': offlineAgents > 0 }">
          <div class="watch-label"><el-icon><Cpu /></el-icon><h2>离线探针</h2></div>
          <div class="watch-value"><strong>{{ offlineAgents ?? '—' }}</strong><span>个探针</span></div>
          <p>{{ !overview ? '等待统计' : `在线 ${summary.probe_agents_online ?? 0} / ${summary.probe_agents ?? 0} 个` }}</p>
        </div>
      </section>

      <div class="workspace-grid">
        <section class="activity-panel" aria-labelledby="activity-title" :aria-busy="loading">
          <header class="panel-heading">
            <div><h2 id="activity-title">运行趋势</h2></div>
            <el-radio-group v-model="range" size="small" aria-label="趋势统计时间范围" @change="loadOverview">
              <el-radio-button value="today">今日</el-radio-button>
              <el-radio-button value="7d">近 7 天</el-radio-button>
              <el-radio-button value="30d">近 30 天</el-radio-button>
            </el-radio-group>
          </header>
          <div class="trend-tabs" role="tablist" aria-label="运行趋势类型">
            <button v-for="(tab, index) in trendTabs" :id="`trend-tab-${tab.id}`" :key="tab.id" type="button" role="tab"
              :aria-selected="activeTrend === tab.id" aria-controls="trend-detail" :tabindex="activeTrend === tab.id ? 0 : -1"
              :class="{ selected: activeTrend === tab.id }" @click="activeTrend = tab.id" @keydown="selectTrendWithKey($event, index)">
              <span><el-icon><component :is="tab.icon" /></el-icon>{{ tab.label }}</span>
              <strong>{{ overview ? tab.count : '—' }}<small>次</small></strong>
            </button>
          </div>
          <div id="trend-detail" v-loading="loading" class="trend-detail" role="tabpanel" :aria-labelledby="`trend-tab-${activeTrend}`">
            <div class="chart-caption"><span>{{ selectedTrend.description }}</span><span class="range-caption">{{ { today: '今日', '7d': '近 7 天', '30d': '近 30 天' }[overview?.range || range] }}</span></div>
            <VChart v-if="overview" class="trend-chart" :option="selectedTrend.option" :update-options="{ notMerge: true }" autoresize :aria-label="`${selectedTrend.label}趋势图`" />
            <el-empty v-else :description="error ? '数据加载失败，请刷新重试' : '等待加载趋势数据'" :image-size="70" />
          </div>
          <div class="quality-summary" aria-label="所选时间范围内的执行质量">
            <div v-for="(item, index) in periodMetrics.slice(1)" :key="item.label" class="quality-item">
              <span>{{ item.label }}</span>
              <strong :class="{ 'has-failures': [period.notifications_failed, period.probe_failed, period.automation_failed][index] > 0 }">{{ overview ? item.value : '—' }}</strong>
              <small>{{ overview ? item.detail : '等待统计' }}</small>
            </div>
          </div>
        </section>

        <aside class="resource-panel" aria-labelledby="resource-title">
          <header class="panel-heading"><div><h2 id="resource-title">资源概览</h2></div><el-icon><FolderOpened /></el-icon></header>
          <div class="datasource-section">
            <div class="datasource-heading"><span>接入数据源</span><strong>{{ summary.datasources ?? '—' }}<small>个</small></strong></div>
            <VChart v-if="summary.datasources > 0" class="datasource-chart" :style="{ height: `${Math.max(128, (overview?.datasource_types?.length || 0) * 24 + 20)}px` }" :option="datasourceOption" autoresize aria-label="接入数据源类型占比环形图" />
            <div v-else class="datasource-empty">{{ overview ? '尚未接入数据源' : '等待加载接入信息' }}</div>
          </div>
          <dl class="inventory-list">
            <div v-for="item in inventory" :key="item.label" class="inventory-row">
              <dt><el-icon><component :is="item.icon" /></el-icon>{{ item.label }}<small v-if="item.detail">{{ item.detail }}</small></dt>
              <dd>{{ item.value ?? '—' }}</dd>
            </div>
          </dl>
        </aside>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { Bell, Tickets, Cpu, Aim, Warning, FolderOpened, Document } from '@element-plus/icons-vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart, BarChart, PieChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import { getDashboardOverview } from '../api/dashboard'
import { useUserStore } from '../stores/user'
import { useThemeColors } from '../theme/useThemeColors'

use([LineChart, BarChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])
const user = useUserStore()
const canView = computed(() => user.hasPermission('dashboard:list'))
const range = ref('today'), overview = ref(null), loading = ref(false), error = ref('')
const colors = useThemeColors()
const tooltipStyle = computed(() => ({ backgroundColor: colors.value.surface, borderColor: colors.value.border, textStyle: { color: colors.value.text } }))
const summary = computed(() => overview.value?.summary || {})
const points = computed(() => overview.value?.points || [])
const alertTotal = computed(() => points.value.reduce((sum, point) => sum + point.alerts, 0))
const notificationTotal = computed(() => points.value.reduce((sum, point) => sum + point.notifications_success + point.notifications_failed, 0))
const period = computed(() => Object.fromEntries(
  ['notifications_success', 'notifications_failed', 'probe_success', 'probe_failed', 'automation_success', 'automation_failed', 'automation_running']
    .map(key => [key, points.value.reduce((total, point) => total + Number(point[key] || 0), 0)]),
))
function successRate(success, failed) {
  return success + failed ? `${(success / (success + failed) * 100).toFixed(1)}%` : '—'
}
const periodMetrics = computed(() => [
  { label: '告警发生', value: alertTotal.value, detail: '所选时间范围内' },
  { label: '通知成功率', value: successRate(period.value.notifications_success, period.value.notifications_failed), detail: `失败 ${period.value.notifications_failed} 次` },
  { label: '拨测成功率', value: successRate(period.value.probe_success, period.value.probe_failed), detail: `失败 ${period.value.probe_failed} 次` },
  { label: '自动化成功率', value: successRate(period.value.automation_success, period.value.automation_failed), detail: `失败 ${period.value.automation_failed} · 执行中 ${period.value.automation_running}（不计入成功率）` },
])
const activeTrend = ref('alerts')
const offlineAgents = computed(() => overview.value ? Math.max(0, (summary.value.probe_agents || 0) - (summary.value.probe_agents_online || 0)) : null)
const trendTabs = computed(() => [
  { id: 'alerts', label: '告警发生', icon: Bell, count: alertTotal.value, option: alertOption.value, description: '按发生时间统计，观察异常集中出现的时段。' },
  { id: 'notifications', label: '通知发送', icon: Tickets, count: notificationTotal.value, option: notificationOption.value, description: '对比发送成功与失败次数，关注通知送达情况。' },
  { id: 'probes', label: '拨测执行', icon: Aim, count: period.value.probe_success + period.value.probe_failed, option: probeOption.value, description: '对比拨测成功与失败次数，观察可用性变化。' },
  { id: 'automation', label: '自动化执行', icon: Cpu, count: period.value.automation_success + period.value.automation_failed + period.value.automation_running, option: automationOption.value, description: '查看任务执行结果，执行中任务不计入成功率。' },
])
const selectedTrend = computed(() => trendTabs.value.find(tab => tab.id === activeTrend.value))
const inventory = computed(() => [
  { label: '告警规则', icon: Tickets, value: summary.value.alert_rules, detail: `启用 ${summary.value.alert_rules_enabled ?? '—'} 条` },
  { label: '拨测任务', icon: Aim, value: summary.value.probe_tasks, detail: `启用 ${summary.value.probe_tasks_enabled ?? '—'} 个` },
  { label: 'Agent 探针', icon: Cpu, value: summary.value.probe_agents, detail: `在线 ${summary.value.probe_agents_online ?? '—'} 个` },
  { label: '规则组', icon: FolderOpened, value: summary.value.alert_rule_groups },
  { label: '故障中心', icon: Warning, value: summary.value.fault_centers },
  { label: '脚本库', icon: Document, value: summary.value.scripts },
])
function selectTrendWithKey(event, index) {
  const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End']
  if (!keys.includes(event.key)) return
  event.preventDefault()
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? 3 : (index + (event.key === 'ArrowRight' ? 1 : -1) + 4) % 4
  activeTrend.value = trendTabs.value[next].id
  event.currentTarget.parentElement.children[next].focus()
}
function trend(series) {
  return {
    color: [colors.value.primary, colors.value.danger], animationDuration: 350,
    tooltip: { ...tooltipStyle.value, trigger: 'axis', renderMode: 'richText' },
    legend: { bottom: 0, textStyle: { color: colors.value.text } },
    grid: { left: 12, right: 16, top: 20, bottom: 42, containLabel: true },
    xAxis: { type: 'category', boundaryGap: series.some(item => item.type === 'bar'), data: points.value.map(p => overview.value?.range === 'today' ? p.time.slice(11) : p.time.slice(5)), axisLine: { lineStyle: { color: colors.value['border-soft'] } }, axisTick: { show: false }, axisLabel: { color: colors.value.muted, fontSize: 11 } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: colors.value.muted, fontSize: 11 }, splitLine: { lineStyle: { color: colors.value['border-soft'], type: 'dashed' } } },
    series,
  }
}
const alertOption = computed(() => trend([{ name: '告警发生次数', type: 'line', showSymbol: points.value.length === 1, symbolSize: 7, lineStyle: { width: 3 }, areaStyle: { opacity: .1 }, data: points.value.map(p => p.alerts) }]))
const notificationOption = computed(() => trend([
  { name: '发送成功', type: 'bar', barMaxWidth: 18, itemStyle: { color: colors.value.success, borderRadius: [3, 3, 0, 0] }, data: points.value.map(p => p.notifications_success) },
  { name: '发送失败', type: 'bar', barMaxWidth: 18, itemStyle: { color: colors.value.danger, borderRadius: [3, 3, 0, 0] }, data: points.value.map(p => p.notifications_failed) },
]))
function executionSeries(name, key, color, type = 'bar') {
  return { name, type, barMaxWidth: 16, showSymbol: false, itemStyle: { color }, data: points.value.map(point => Number(point[key] || 0)) }
}
const probeOption = computed(() => trend([
  executionSeries('拨测成功', 'probe_success', colors.value.success, 'line'),
  executionSeries('拨测失败', 'probe_failed', colors.value.danger, 'line'),
]))
const automationOption = computed(() => trend([
  executionSeries('执行成功', 'automation_success', colors.value.success),
  executionSeries('执行失败', 'automation_failed', colors.value.danger),
  executionSeries('执行中', 'automation_running', colors.value.primary),
]))
const datasourceTypeNames = {
  prometheus: 'Prometheus', victoriametrics: 'VictoriaMetrics', loki: 'Loki',
  elasticsearch: 'Elasticsearch', clickhouse: 'ClickHouse', jaeger: 'Jaeger',
}
const datasourceOption = computed(() => ({
  color: [colors.value.primary, '#8a69d8', '#26a98b', '#de884b', '#e85d75', '#5686ec'],
  tooltip: { ...tooltipStyle.value, trigger: 'item', renderMode: 'richText', formatter: '{b}: {c} 个 ({d}%)' },
  legend: { type: 'plain', orient: 'vertical', left: '43%', right: 0, top: 'middle', itemWidth: 10, itemHeight: 10, itemGap: 10, textStyle: { color: colors.value.text, fontSize: 11, lineHeight: 14 } },
  series: [{
    type: 'pie', radius: [27, 40], center: ['20%', '50%'],
    label: { show: false },
    itemStyle: { borderColor: colors.value.surface, borderWidth: 2 },
    data: (overview.value?.datasource_types || []).map(item => ({
      name: datasourceTypeNames[item.name] || item.name,
      value: item.count,
    })),
  }],
}))
let requestID = 0, refreshTimer
async function loadOverview() {
  if (!canView.value) return
  const id = ++requestID
  loading.value = true
  error.value = ''
  try {
    const result = await getDashboardOverview(range.value)
    if (id === requestID) overview.value = result.data
  } catch {
    if (id === requestID) { overview.value = null; error.value = '统计数据加载失败，请点击刷新重试。' }
  } finally { if (id === requestID) loading.value = false }
}
onMounted(() => {
  loadOverview()
  refreshTimer = setInterval(() => { if (!document.hidden && !loading.value) loadOverview() }, 60000)
})
onBeforeUnmount(() => { requestID++; clearInterval(refreshTimer) })
</script>

<style scoped>
.dashboard { width: 100%; min-width: 0; box-sizing: border-box; margin: 0; display: flex; flex-direction: column; gap: 24px; padding: 0 0 20px; color: var(--ops-text); }
.workspace-heading, .panel-heading { display: flex; align-items: center; justify-content: space-between; gap: 18px; }
.workspace-heading h1 { margin: 0; font-size: 25px; line-height: 1.4; font-weight: 650; letter-spacing: -.5px; }
.workspace-heading p { margin: 7px 0 0; font-size: 13px; color: var(--ops-muted); }
.watch-summary { display: grid; grid-template-columns: 1.35fr 1fr 1fr; gap: 20px; }
.alert-focus { --tile-accent: #7361e9; position: relative; padding: 24px 28px; }
.alert-focus.has-alerts { --tile-accent: var(--ops-danger); }
.focus-label, .watch-label { display: flex; align-items: center; gap: 9px; font-size: 13px; }
.focus-label .el-icon { color: var(--ops-primary); font-size: 17px; }
.has-alerts .focus-label .el-icon, .has-alerts .focus-value strong { color: var(--ops-danger); }
.focus-value { display: flex; align-items: baseline; gap: 10px; margin: 10px 0; }
.focus-value strong { font-size: 48px; line-height: 1.1; letter-spacing: -1.5px; font-weight: 600; font-variant-numeric: tabular-nums; }
.focus-value > span, .watch-value > span { color: var(--ops-muted); font-size: 12px; }
.alert-focus p, .watch-item p { margin: 0; color: var(--ops-muted); font-size: 12px; line-height: 1.7; }
.watch-item { --tile-accent: #008eac; position: relative; padding: 24px 26px; }
.watch-item + .watch-item { --tile-accent: #b267d2; }
.watch-label { color: var(--ops-muted); }
.watch-label h2 { margin: 0; color: var(--ops-text); font-size: 13px; font-weight: 500; }
.watch-value { display: flex; align-items: baseline; gap: 9px; margin: 12px 0; }
.watch-value strong { font-size: 34px; line-height: 1.2; font-weight: 550; font-variant-numeric: tabular-nums; }
.watch-item.is-warning .watch-value strong, .watch-item.is-warning .watch-label .el-icon { color: var(--ops-danger); }
.workspace-grid { display: grid; grid-template-columns: minmax(0, 1fr) clamp(300px, 23%, 420px); gap: 24px; align-items: start; }
.activity-panel, .resource-panel { min-width: 0; border: 1px solid var(--ops-border-soft); border-radius: 20px; background: color-mix(in srgb, var(--ops-surface) 94%, transparent); box-shadow: 0 8px 32px #3744a008; backdrop-filter: blur(14px); overflow: hidden; }
.panel-heading { padding: 24px; }
.panel-heading h2 { margin: 0; font-size: 16px; font-weight: 600; }
.panel-heading p { margin: 6px 0 0; color: var(--ops-muted); font-size: 12px; }
.trend-tabs { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); padding: 0 24px; border-bottom: 1px solid var(--ops-border-soft); gap: 8px; }
.trend-tabs button { appearance: none; position: relative; border: 0; border-bottom: 3px solid transparent; background: transparent; color: var(--ops-muted); text-align: left; padding: 15px 12px 16px; font-family: inherit; cursor: pointer; border-radius: 7px 7px 0 0; }
.trend-tabs button > span { display: flex; align-items: center; gap: 7px; font-size: 12px; white-space: nowrap; }
.trend-tabs button > strong { display: block; color: var(--ops-text); font-size: 24px; font-weight: 550; margin-top: 10px; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.trend-tabs small { margin-left: 6px; font-size: 11px; font-weight: 400; color: var(--ops-muted); }
.trend-tabs button.selected { background: color-mix(in srgb, var(--tab-accent) 13%, var(--ops-surface)); color: var(--tab-accent); border-bottom-color: var(--tab-accent); box-shadow: inset 0 1px 0 color-mix(in srgb, var(--tab-accent) 18%, transparent); }
.trend-tabs button.selected strong { color: var(--tab-accent); }
.trend-tabs button:hover:not(.selected) { background: var(--ops-surface-soft); }
.trend-tabs button:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
.trend-detail { padding: 22px 24px 12px; min-height: 344px; }
.chart-caption { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; color: var(--ops-muted); font-size: 12px; line-height: 1.7; }
.range-caption { flex-shrink: 0; color: var(--ops-subtle); font-size: 11px; }
.trend-chart { height: 285px; margin-top: 12px; }
.trend-detail :deep(.el-empty) { min-height: 285px; }
.quality-summary { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 22px; padding: 22px 24px; background: var(--ops-surface-soft); border-top: 1px solid var(--ops-border-soft); }
.quality-item { display: flex; flex-direction: column; gap: 8px; }
.quality-item > span { font-size: 12px; color: var(--ops-muted); }
.quality-item strong { font-size: 23px; font-weight: 550; font-variant-numeric: tabular-nums; }
.quality-item .has-failures { color: var(--ops-danger); }
.quality-item small { color: var(--ops-muted); font-size: 11px; line-height: 1.7; }
.resource-panel .panel-heading { border-bottom: 1px solid var(--ops-border-soft); }
.resource-panel .panel-heading > .el-icon { color: var(--ops-subtle); font-size: 19px; }
.datasource-section { padding: 20px 22px 12px; }
.datasource-heading { display: flex; align-items: center; justify-content: space-between; gap: 10px; font-size: 12px; color: var(--ops-muted); }
.datasource-heading strong { font-size: 23px; color: var(--ops-text); font-weight: 550; font-variant-numeric: tabular-nums; }
.datasource-heading small { margin-left: 6px; font-size: 11px; font-weight: 400; color: var(--ops-muted); }
.datasource-chart { height: 128px; margin-top: 6px; }
.datasource-empty { min-height: 128px; display: grid; place-items: center; font-size: 12px; color: var(--ops-muted); }
.inventory-list { margin: 0; padding: 0 22px 10px; }
.inventory-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 16px 0; border-top: 1px solid var(--ops-border-soft); }
.inventory-row dt { display: grid; grid-template-columns: 16px 1fr; align-items: center; gap: 5px 9px; font-size: 12px; }
.inventory-row dt > .el-icon { color: var(--ops-subtle); font-size: 15px; }
.inventory-row dt small { grid-column: 2; color: var(--ops-muted); font-size: 11px; }
.inventory-row dd { margin: 0; font-size: 19px; font-weight: 500; font-variant-numeric: tabular-nums; }

.workspace-heading { position: relative; isolation: isolate; overflow: hidden; min-height: 270px; box-sizing: border-box; padding: 38px 40px; border: 1px solid #d8ddf4; border-radius: 22px; color: #34416b; background: radial-gradient(ellipse at 68% 110%, #d4bef57a, transparent 48%), radial-gradient(ellipse at 76% 16%, #b7ebf599, transparent 42%), linear-gradient(115deg, #f8faff 0%, #eef0ff 45%, #f4efff 100%); box-shadow: 0 12px 32px #7986c518, inset 0 1px 0 #ffffff; }
.workspace-heading::after { content: ''; position: absolute; bottom: 0; left: 12%; right: 12%; height: 1px; background: linear-gradient(90deg, transparent, #8b8bff, #73e9f9, transparent); box-shadow: 0 -2px 18px #8687ff; pointer-events: none; }
.workspace-intro { position: relative; z-index: 1; width: 43%; max-width: 520px; }
.workspace-badge { display: inline-flex; align-items: center; gap: 12px; font-size: clamp(18px, 1.6vw, 24px); font-weight: 600; letter-spacing: 1px; color: #3f487e; margin-bottom: 12px; text-shadow: 0 2px 16px #ffffff90; }
.workspace-badge > span { width: 9px; height: 9px; flex-shrink: 0; border-radius: 3px; transform: rotate(45deg); background: #6382e9; box-shadow: 0 0 0 5px #84dfff12, 0 0 20px #7ce9ff90; }
.workspace-heading p { max-width: 390px; color: #626e90; font-size: 14px; line-height: 1.9; margin: 10px 0 0; }
.workspace-tags { display: flex; flex-wrap: wrap; gap: 10px; margin-top: 26px; color: #53658e; font-size: 11px; }
.workspace-tags > span { display: flex; align-items: center; gap: 7px; padding: 8px 10px; border: 1px solid #ffffff; border-radius: 7px; background: linear-gradient(135deg, #ffffffcc, #ffffff55); box-shadow: inset 0 1px 0 #ffffff0a; }
.workspace-tags .el-icon { font-size: 15px; color: #2586a7; }
.workspace-tags > span:nth-child(2) .el-icon { color: #8560c4; }
.workspace-tags > span:nth-child(3) .el-icon { color: #b564a4; }
.orbital-art { position: absolute; left: 66%; top: 50%; width: 380px; height: 270px; transform: translate(-50%, -50%); pointer-events: none; animation: orbit-reveal 1.6s ease-out both; }
.orbit-halo { position: absolute; inset: 10% 12%; border-radius: 50%; background: radial-gradient(ellipse, #7885ff40, #72a5ef08 58%, transparent 72%); filter: blur(12px); }
.orbit-floor { position: absolute; left: -25%; right: -25%; height: 200px; bottom: -100px; transform: perspective(240px) rotateX(62deg); transform-origin: top; background-image: linear-gradient(#8b9aff25 1px, transparent 1px), linear-gradient(90deg, #8b9aff25 1px, transparent 1px); background-size: 28px 28px; mask-image: radial-gradient(ellipse at 50% 0, black, transparent 70%); }
.signal-trails { position: absolute; width: 116%; height: 100%; left: -8%; top: 0; overflow: visible; }
.signal-trails path { stroke: #8fa5db55; stroke-width: 1; }
.signal-trails path.signal-highlight { stroke: #609eda; stroke-dasharray: 40 350; filter: drop-shadow(0 0 4px #63bfff); animation: signal-arrive 2.8s ease-out both; }
.orbit { position: absolute; inset: 32px 40px; border: 1px solid #8d9ce4a0; border-radius: 50%; transform: rotate(-22deg) scaleY(.58); box-shadow: 0 0 12px #6e95ff25, inset 0 0 10px #6e95ff15; }
.orbit-two { inset: 39px 46px; transform: rotate(40deg) scaleY(.72); border-color: #75bfcda0; border-bottom: 2px solid #73bbd4; }
.orbit-three { inset: 24px 57px; transform: rotate(-62deg) scaleY(.7); border-color: #aa8bd5a0; border-right: 2px solid #ad8be2; }
.orbit-core { position: absolute; width: 94px; height: 94px; left: calc(50% - 47px); top: calc(50% - 47px); display: grid; place-items: center; border: 1px solid #ffffff; border-radius: 24px; background: linear-gradient(135deg, #ffffffee, #b8cafa 45%, #d7b9ec); box-shadow: 0 0 0 7px #b0bfff0b, 0 0 0 15px #a1bcff08, 0 0 50px #8e9bff70, inset 3px 3px 12px #ffffff40, inset -4px -4px 12px #8c88c035; transform: rotate(-12deg); }
.core-inner { display: grid; place-items: center; width: 65px; height: 65px; color: #6677c5; font-size: 40px; border: 1px solid #ffffffaa; border-radius: 17px; background: linear-gradient(145deg, #ffffffb0, #e0e7ff80); text-shadow: 0 0 12px #c0eaff; }
.core-robot { filter: drop-shadow(0 0 7px #b9edff80); }
.orbit-node { position: absolute; display: flex; align-items: center; gap: 8px; padding: 9px 12px; border: 1px solid #ffffff; border-radius: 10px; color: #478ab3; background: linear-gradient(135deg, #ffffffed, #dceefbed); box-shadow: 0 8px 22px #6b7cbd25, inset 0 1px 0 #ffffff20; font-size: 20px; }
.orbit-node small { font-size: 10px; color: #596184; letter-spacing: 1px; }
.node-one { left: 16px; top: 55px; transform: rotate(-8deg); }
.node-two { right: 6px; top: 111px; transform: rotate(8deg); color: #8965c5; background: linear-gradient(135deg, #ffffffee, #e7dffbed); }
.node-three { bottom: 24px; left: 103px; transform: rotate(-5deg); color: #af70a7; background: linear-gradient(135deg, #ffffffee, #f1dff1ed); }
@keyframes signal-arrive { from { stroke-dashoffset: 390; opacity: 0; } to { stroke-dashoffset: 0; opacity: 1; } }
.alert-focus, .watch-item { overflow: hidden; border: 1px solid color-mix(in srgb, var(--tile-accent) 22%, var(--ops-surface)); border-radius: 18px; background: radial-gradient(ellipse at 100% 0, color-mix(in srgb, var(--tile-accent) 13%, transparent), transparent 68%), var(--ops-surface); box-shadow: 0 8px 24px color-mix(in srgb, var(--tile-accent) 7%, transparent); }
.alert-focus::after, .watch-item::after { content: ''; position: absolute; width: 105px; height: 105px; right: -38px; bottom: -46px; border-radius: 50%; border: 14px solid color-mix(in srgb, var(--tile-accent) 7%, transparent); pointer-events: none; }
.focus-label .el-icon, .watch-label .el-icon { display: grid; place-items: center; width: 30px; height: 30px; border-radius: 9px; color: var(--tile-accent); background: color-mix(in srgb, var(--tile-accent) 12%, var(--ops-surface)); font-size: 17px; }
.focus-value strong { color: var(--tile-accent); font-size: 44px; }
.watch-value strong { color: var(--tile-accent); font-size: 38px; }
.watch-item.is-warning { --tile-accent: var(--ops-danger); }
.panel-heading h2 { display: flex; align-items: center; gap: 9px; }
.panel-heading h2::before { content: ''; width: 4px; height: 16px; border-radius: 4px; background: linear-gradient(#9473e9, #5878e6); box-shadow: 0 0 9px #8c6be633; }
.trend-tabs button { --tab-accent: var(--ops-primary); transition: background .18s ease, box-shadow .18s ease; border-radius: 12px 12px 0 0; }
.trend-tabs button:nth-child(2) { --tab-accent: var(--ops-success); }
.trend-tabs button:nth-child(3) { --tab-accent: light-dark(#7154bf, #bea3ff); }
.trend-tabs button:nth-child(4) { --tab-accent: light-dark(#ae5b9e, #e8a0d8); }
.quality-summary { background: linear-gradient(110deg, color-mix(in srgb, #6a79e8 5%, var(--ops-surface)), color-mix(in srgb, #b976dd 5%, var(--ops-surface))); }
.quality-item strong { color: var(--ops-primary); }
.quality-item:nth-child(2) strong { color: light-dark(#7154bf, #bea3ff); }
.quality-item:nth-child(3) strong { color: var(--ops-success); }
.quality-item strong.has-failures { color: var(--ops-danger); }
.resource-panel .panel-heading { background: linear-gradient(115deg, color-mix(in srgb, #755cdd 9%, var(--ops-surface)), color-mix(in srgb, #3ba6db 6%, var(--ops-surface))); }
.resource-panel .panel-heading > .el-icon { padding: 10px; border-radius: 12px; color: var(--ops-primary); background: var(--ops-surface); box-shadow: 0 4px 12px #675aca0a; }
.inventory-row dt > .el-icon { background: var(--ops-primary-soft); color: var(--ops-primary); border-radius: 6px; padding: 4px; margin-left: -4px; }
.inventory-row:nth-child(2n) dt > .el-icon { color: light-dark(#7154bf, #bea3ff); background: color-mix(in srgb, #9874e0 12%, var(--ops-surface)); }
.inventory-row dt { column-gap: 15px; }
@keyframes orbit-reveal { from { opacity: 0; transform: translate(-50%, -45%) scale(.88); } to { opacity: 1; transform: translate(-50%, -50%) scale(1); } }
@media (prefers-reduced-motion: reduce) { .orbital-art, .signal-trails path.signal-highlight { animation: none; } .trend-tabs button { transition: none; } }
@media (max-width: 1200px) {
  .workspace-grid { grid-template-columns: minmax(0, 1fr) 260px; gap: 18px; }
  .orbital-art { left: 65%; width: 310px; }
  .workspace-heading { padding: 30px; }
  .workspace-intro { width: 44%; }
  .watch-summary { gap: 14px; }
  .watch-item { padding: 24px 22px; }
  .panel-heading { flex-wrap: wrap; padding: 20px; }
  .trend-tabs { padding: 0 16px; gap: 4px; }
  .trend-tabs button { padding-left: 8px; padding-right: 8px; }
  .trend-tabs button > span { gap: 5px; }
  .trend-tabs button .el-icon { display: none; }
}
@media (max-width: 960px) {
  .workspace-grid { grid-template-columns: minmax(0, 1fr); }
  .resource-panel { display: grid; grid-template-columns: 1fr 1.4fr; }
  .resource-panel .panel-heading { grid-column: 1 / -1; }
  .inventory-list { padding-top: 6px; }
  .inventory-row { padding: 12px 0; }
  .inventory-row:first-child { border-top: 0; }
  .trend-tabs button .el-icon { display: inline-flex; }
  .orbital-art { left: 78%; width: 310px; opacity: .5; }
  .workspace-intro { width: 60%; }
  .alert-focus { padding: 22px; }
  .watch-item { padding: 22px 18px; }
}
@media (max-width: 600px) {
  .dashboard { gap: 18px; padding: 0 0 12px; }
  .workspace-heading { padding: 24px 20px; min-height: 240px; }
  .workspace-heading .workspace-intro { max-width: 100%; width: 100%; }
  .workspace-tags { gap: 12px; font-size: 10px; flex-wrap: wrap; }
  .orbital-art { display: none; }
  .workspace-badge { font-size: 19px; letter-spacing: 0; }
  .workspace-heading { align-items: flex-start; flex-direction: column; gap: 18px; }
  .workspace-heading h1 { font-size: 27px; }
  .workspace-heading p { font-size: 12px; max-width: 320px; line-height: 1.8; }
  .watch-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .alert-focus { grid-column: 1 / -1; }
  .watch-item { padding: 20px; }
  .watch-value strong { font-size: 29px; }
  .watch-item p { font-size: 11px; }
  .trend-tabs { padding: 0 12px; gap: 0; }
  .trend-tabs button { padding: 12px 6px; }
  .trend-tabs button .el-icon { display: none; }
  .trend-tabs button > span { font-size: 11px; }
  .trend-tabs button > strong { font-size: 22px; }
  .trend-detail { padding: 18px 12px 8px; }
  .chart-caption { font-size: 11px; }
  .quality-summary { padding: 18px 16px; gap: 14px; }
  .quality-item strong { font-size: 20px; }
  .quality-item > span { font-size: 11px; }
  .resource-panel { grid-template-columns: minmax(0, 1fr); }
  .datasource-chart { max-width: 340px; }
}
</style>

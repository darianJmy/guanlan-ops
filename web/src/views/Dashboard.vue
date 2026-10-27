<template>
  <div class="dashboard">
    <el-empty v-if="!canView" description="暂无查看运维概览的权限，请联系管理员授权" />
    <template v-else>
      <div class="overview-toolbar">
        <div class="overview-controls">
          <el-radio-group v-model="range" size="small" @change="loadOverview">
            <el-radio-button value="today">今日</el-radio-button>
            <el-radio-button value="7d">近 7 天</el-radio-button>
            <el-radio-button value="30d">近 30 天</el-radio-button>
          </el-radio-group>
          <el-button :icon="Refresh" :loading="loading" size="small" @click="loadOverview">刷新</el-button>
        </div>
      </div>
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
      <div v-loading="loading" class="overview-content" :aria-busy="loading">
        <div class="metric-grid">
          <section v-for="card in cards" :key="card.label" class="metric-card">
            <div class="metric-top"><span>{{ card.label }}</span><el-icon :class="card.tone"><component :is="card.icon" /></el-icon></div>
            <strong>{{ card.value }}</strong>
          </section>
        </div>
        <div class="chart-grid">
          <section class="chart-card">
            <div class="chart-heading"><div><h2>告警发生趋势</h2></div><span class="chart-total">{{ overview ? alertTotal : '—' }} <small>次</small></span></div>
            <VChart v-if="overview" class="trend-chart" :option="alertOption" autoresize aria-label="告警发生趋势折线图" />
            <el-empty v-else description="暂无统计数据" :image-size="72" />
          </section>
          <section class="chart-card">
            <div class="chart-heading"><div><h2>通知发送趋势</h2></div><span class="chart-total">{{ overview ? notificationTotal : '—' }} <small>次</small></span></div>
            <VChart v-if="overview" class="trend-chart" :option="notificationOption" autoresize aria-label="通知发送成功与失败趋势图" />
            <el-empty v-else description="暂无统计数据" :image-size="72" />
          </section>
          <section class="chart-card">
            <div class="chart-heading"><div><h2>当前活跃告警</h2></div><span class="chart-total">{{ summary.active_alerts ?? '—' }} <small>条</small></span></div>
            <VChart v-if="overview?.severity.length" class="distribution-chart" :option="severityOption" autoresize aria-label="当前活跃告警严重程度分布" />
            <el-empty v-else :description="overview ? '当前没有活跃告警' : '暂无统计数据'" :image-size="80" />
          </section>
          <section class="chart-card">
            <div class="chart-heading"><div><h2>数据源接入状态</h2></div><span class="chart-total">{{ summary.datasources ?? '—' }} <small>个</small></span></div>
            <VChart v-if="summary.datasources > 0" class="distribution-chart" :option="datasourceOption" autoresize aria-label="数据源实例启用状态" />
            <el-empty v-else :description="overview ? '尚未接入数据源' : '暂无统计数据'" :image-size="80" />
          </section>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { Refresh, Bell, Monitor, Cpu, Aim } from '@element-plus/icons-vue'
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
const cards = computed(() => [
  { label: '数据源接入', value: summary.value.datasources ?? '—', icon: Monitor, tone: 'purple' },
  { label: 'Agent 探针', value: summary.value.probe_agents ?? '—', icon: Cpu, tone: 'green' },
  { label: '拨测任务', value: summary.value.probe_tasks ?? '—', icon: Aim, tone: 'blue' },
  { label: '今日发生告警', value: summary.value.today_alerts ?? '—', icon: Bell, tone: 'orange' },
])
function trend(series) {
  return {
    color: [colors.value.primary, colors.value.danger], animationDuration: 350,
    tooltip: { ...tooltipStyle.value, trigger: 'axis', renderMode: 'richText' },
    legend: { bottom: 0, textStyle: { color: colors.value.text } },
    grid: { left: 12, right: 16, top: 20, bottom: 42, containLabel: true },
    xAxis: { type: 'category', boundaryGap: false, data: points.value.map(p => range.value === 'today' ? p.time.slice(11) : p.time.slice(5)), axisLine: { lineStyle: { color: colors.value['border-soft'] } }, axisTick: { show: false }, axisLabel: { color: colors.value.text } },
    yAxis: { type: 'value', minInterval: 1, axisLabel: { color: colors.value.text }, splitLine: { lineStyle: { color: colors.value['border-soft'], type: 'dashed' } } },
    series,
  }
}
const alertOption = computed(() => trend([{ name: '告警发生次数', type: 'line', showSymbol: points.value.length === 1, symbolSize: 7, lineStyle: { width: 3 }, areaStyle: { opacity: .1 }, data: points.value.map(p => p.alerts) }]))
const notificationOption = computed(() => trend([
  { name: '发送成功', type: 'line', showSymbol: points.value.length === 1, itemStyle: { color: colors.value.success }, areaStyle: { opacity: .06 }, data: points.value.map(p => p.notifications_success) },
  { name: '发送失败', type: 'line', showSymbol: points.value.length === 1, itemStyle: { color: colors.value.danger }, data: points.value.map(p => p.notifications_failed) },
]))
const severityOption = computed(() => ({
  color: ['#e85d75', '#ee9362', '#eab649', '#5686ec', '#58b5a5'],
  tooltip: { ...tooltipStyle.value, trigger: 'item', renderMode: 'richText' }, legend: { bottom: 0, textStyle: { color: colors.value.text } },
  series: [{ type: 'pie', radius: ['46%', '69%'], center: ['50%', '45%'], avoidLabelOverlap: true, label: { color: colors.value.text, formatter: '{b}: {c}' }, data: (overview.value?.severity || []).map(item => ({ name: item.name || '未分类', value: item.count })) }],
}))
const datasourceOption = computed(() => ({
  color: [colors.value.primary], tooltip: { ...tooltipStyle.value, trigger: 'axis', renderMode: 'richText', axisPointer: { type: 'shadow' } },
  grid: { left: 12, right: 28, top: 14, bottom: 12, containLabel: true },
  xAxis: { type: 'value', minInterval: 1, axisLabel: { color: colors.value.text }, splitLine: { lineStyle: { color: colors.value['border-soft'] } } },
  yAxis: { type: 'category', inverse: true, data: ['启用', '禁用'], axisLabel: { color: colors.value.text }, axisTick: { show: false }, axisLine: { show: false } },
  series: [{ type: 'bar', barMaxWidth: 22, itemStyle: { borderRadius: [0, 5, 5, 0] }, label: { show: true, position: 'right', color: colors.value.text }, data: [Number(summary.value.datasources_enabled || 0), Math.max(0, Number(summary.value.datasources || 0) - Number(summary.value.datasources_enabled || 0))] }],
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
.dashboard,.overview-content{display:flex;flex-direction:column;gap:18px;min-width:0}.overview-toolbar{display:flex;align-items:center;justify-content:flex-end;gap:12px;flex-wrap:wrap}.overview-controls{display:flex;align-items:center;gap:10px}.metric-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:16px}.metric-card,.chart-card{background:var(--ops-surface);border:1px solid var(--ops-border-soft);border-radius:10px}.metric-card{padding:20px}.metric-top{display:flex;align-items:center;justify-content:space-between;gap:8px;color:var(--ops-muted);font-size:13px}.metric-top .el-icon{padding:9px;border-radius:10px;font-size:20px}.blue{color:var(--ops-primary);background:var(--ops-primary-soft)}.purple{color:#8a69d8;background:#8a69d81a}.orange{color:#de884b;background:#de884b1a}.green{color:#26a98b;background:#26a98b1a}.metric-card strong{display:block;font-size:32px;line-height:1.4;margin:8px 0;color:var(--ops-text);font-variant-numeric:tabular-nums}.chart-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}.chart-card{min-width:0;padding:20px}.chart-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:12px}.chart-heading h2{margin:0;font-size:15px;color:var(--ops-text);font-weight:650}.chart-total{flex:none;color:var(--ops-text);font-size:25px;font-weight:650;font-variant-numeric:tabular-nums}.chart-total small{font-size:11px;font-weight:400;color:var(--ops-muted)}.trend-chart{height:280px;margin-top:14px}.distribution-chart{height:270px;margin-top:14px}@media(max-width:1200px){.metric-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:850px){.chart-grid{grid-template-columns:1fr}.chart-card{padding:16px}}@media(max-width:480px){.metric-grid{gap:10px}.metric-card{padding:14px}.metric-top .el-icon{display:none}.metric-card strong{font-size:27px}.overview-controls{flex-wrap:wrap}}
</style>

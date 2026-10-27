<template>
  <div class="detail-page" v-loading="loading">
    <header class="detail-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="router.push(item.group_id ? `/inspection/items/groups/${item.group_id}` : '/inspection/items')">返回</el-button>
      <div class="toolbar-actions">
        <el-select v-model="selectedNodeKey" aria-label="执行节点筛选" @change="changeNode"><el-option v-for="node in nodes" :key="node.node_key" :label="node.node_name" :value="node.node_key" /></el-select>
        <el-select v-model="rangeKey" aria-label="趋势时间范围" @change="loadData"><el-option v-for="option in ranges" :key="option.value" :label="option.label" :value="option.value" /></el-select>
        <el-button :icon="Edit" @click="router.push(`/inspection/items/${route.params.id}/edit`)" v-permission="['inspection:edit']">编辑</el-button>
      </div>
    </header>

    <main class="chart-grid">
      <section class="chart-card">
        <div class="card-heading"><div><strong>可用率趋势</strong><span>按时间窗口统计拨测成功占比</span></div><span class="range-note">{{ currentRangeLabel }}</span></div>
        <div class="chart-shell"><v-chart v-if="hasTrendData" class="chart" :option="availabilityOption" autoresize /><el-empty v-else :image-size="72" description="当前时间范围内暂无拨测数据" /></div>
      </section>

      <section class="chart-card">
        <div class="card-heading"><div><strong>执行耗时趋势</strong><span>每次拨测的端到端执行耗时</span></div><span class="range-note">毫秒</span></div>
        <div class="chart-shell"><v-chart v-if="hasTrendData" class="chart" :option="durationOption" autoresize /><el-empty v-else :image-size="72" description="当前时间范围内暂无耗时数据" /></div>
      </section>

      <section v-if="isHTTP" class="chart-card full-width">
        <div class="card-heading"><div><strong>HTTP 状态码趋势</strong><span>按时间窗口统计 2xx、3xx、4xx 与 5xx 响应数量</span></div><span class="range-note">请求次数</span></div>
        <div class="chart-shell"><v-chart v-if="httpPointCount" class="chart wide-chart" :option="httpStatusOption" autoresize /><el-empty v-else :image-size="72" description="当前时间范围内暂无 HTTP 状态码数据" /></div>
      </section>
    </main>

    <section class="records-card">
      <button type="button" class="card-heading records-toggle" :class="{ open: recordsExpanded }" :aria-expanded="recordsExpanded" aria-controls="recent-probe-table" @click="recordsExpanded = !recordsExpanded">
        <div><strong>最近拨测</strong></div>
        <span class="records-meta"><span class="range-note">共 {{ recordTotal }} 条</span><el-icon class="toggle-icon"><ArrowDown /></el-icon></span>
      </button>
      <el-collapse-transition>
        <div v-show="recordsExpanded" id="recent-probe-table">
          <el-table :data="recentPoints" stripe>
            <el-table-column label="状态" width="100"><template #default="{ row }"><el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">{{ row.status === 'success' ? '正常' : '失败' }}</el-tag></template></el-table-column>
            <el-table-column v-if="item.type === 'script'" label="退出码" width="90"><template #default="{ row }">{{ parseObject(row.result).exit_code ?? '—' }}</template></el-table-column>
            <el-table-column v-if="isHTTP" label="HTTP 状态码" width="130"><template #default="{ row }"><span :class="`status-code code-text-${statusGroup(recordStatusCode(row))}`">{{ recordStatusCode(row) || '—' }}</span></template></el-table-column>
            <el-table-column label="请求耗时" width="130"><template #default="{ row }"><el-tag class="duration-tag" type="info" effect="plain" size="small">{{ row.duration_ms }} ms</el-tag></template></el-table-column>
            <el-table-column label="结果详情" min-width="360"><template #default="{ row }"><CertificateExpiryResult v-if="item.type === 'certificate_expiry'" :result="row.result" /><ScriptProbeResult v-else-if="item.type === 'script'" :result="row.result" /><code v-else class="record-result">{{ formatResult(row.result) }}</code></template></el-table-column>
            <el-table-column label="执行时间" min-width="180"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
            <template #empty><TableEmpty /></template>
          </el-table>
        </div>
      </el-collapse-transition>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowDown, ArrowLeft, Edit } from '@element-plus/icons-vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { DataZoomComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import ScriptProbeResult from '../../components/ScriptProbeResult.vue'
import CertificateExpiryResult from '../../components/CertificateExpiryResult.vue'
import { getInspectionItemOverview, getInspectionRecords } from '../../api/inspection'

use([LineChart, DataZoomComponent, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer])

const route = useRoute(), router = useRouter()
const loading = ref(true), rangeKey = ref('24h')
const overview = ref({ item: {}, summary: {}, points: [] })
const recentPoints = ref([]), recordTotal = ref(0)
const recordsExpanded = ref(false)
const ranges = [{ label: '最近 1 小时', value: '1h', ms: 3600000 }, { label: '最近 6 小时', value: '6h', ms: 21600000 }, { label: '最近 24 小时', value: '24h', ms: 86400000 }, { label: '最近 7 天', value: '7d', ms: 604800000 }, { label: '最近 30 天', value: '30d', ms: 2592000000 }]
const item = computed(() => overview.value.item || {})
const points = computed(() => overview.value.points || [])
const nodes = computed(() => overview.value.nodes || [])
const selectedNodeKey = ref('')
const selectedNode = computed(() => nodes.value.find(node => node.node_key === selectedNodeKey.value) || null)
const visiblePoints = computed(() => selectedNode.value ? points.value.filter(point => point.node_type === selectedNode.value.node_type && Number(point.node_id) === Number(selectedNode.value.node_id)) : [])
const isHTTP = computed(() => item.value.type === 'http')
const codeCounts = computed(() => visiblePoints.value.reduce((result, point) => { result['2xx'] += Number(point.status_2xx_count || 0); result['3xx'] += Number(point.status_3xx_count || 0); result['4xx'] += Number(point.status_4xx_count || 0); result['5xx'] += Number(point.status_5xx_count || 0); return result }, { '2xx': 0, '3xx': 0, '4xx': 0, '5xx': 0 }))
const currentRange = computed(() => ranges.find(entry => entry.value === rangeKey.value) || ranges[2])
const currentRangeLabel = computed(() => currentRange.value.label)
const hasTrendData = computed(() => visiblePoints.value.some(point => Number(point.total_count || 0) > 0))
const httpPointCount = computed(() => Object.values(codeCounts.value).reduce((sum, value) => sum + value, 0))

const availabilityOption = computed(() => lineOption({
  labels: visiblePoints.value.map(point => formatAxisTime(point.created_at)),
  yName: '%', min: 0, max: 100,
  series: [{ name: '可用率', data: visiblePoints.value.map(point => point.total_count ? Number((point.success_count * 100 / point.total_count).toFixed(1)) : null), color: '#18a078', area: true }],
  tooltipSuffix: '%',
}))

const durationOption = computed(() => lineOption({
  labels: visiblePoints.value.map(point => formatAxisTime(point.created_at)),
  yName: 'ms', min: 0,
  series: [{ name: '请求耗时', data: visiblePoints.value.map(point => point.total_count ? point.duration_ms : null), color: '#2878e8', area: true }],
  tooltipSuffix: ' ms', zoom: visiblePoints.value.length > 40,
}))

const httpStatusOption = computed(() => lineOption({
  labels: visiblePoints.value.map(point => formatAxisTime(point.created_at)), yName: '次', min: 0,
  series: [
    { name: '2xx', data: visiblePoints.value.map(point => point.status_2xx_count), color: '#18a078' },
    { name: '3xx', data: visiblePoints.value.map(point => point.status_3xx_count), color: '#2878e8' },
    { name: '4xx', data: visiblePoints.value.map(point => point.status_4xx_count), color: '#e6a23c' },
    { name: '5xx', data: visiblePoints.value.map(point => point.status_5xx_count), color: '#e25555' },
  ], legend: true, tooltipSuffix: ' 次',
}))

onMounted(loadData)

async function loadData() {
  loading.value = true
  try {
    const overviewParams = { range: rangeKey.value }
    if (selectedNode.value) Object.assign(overviewParams, { node_type: selectedNode.value.node_type, node_id: selectedNode.value.node_id })
    const overviewRes = await getInspectionItemOverview(route.params.id, overviewParams)
    overview.value = overviewRes.data || { item: {}, summary: {}, points: [] }
    selectedNodeKey.value = overview.value.selected_node_key || nodes.value[0]?.node_key || ''
    await loadNodeRecords()
  } catch {
    router.push('/inspection/items')
  } finally { loading.value = false }
}

async function changeNode() {
  await loadData()
}

async function loadNodeRecords() {
  const params = { item_id: route.params.id, page: 1, page_size: 12 }
  if (selectedNode.value) Object.assign(params, { node_type: selectedNode.value.node_type, node_id: selectedNode.value.node_id })
  const response = await getInspectionRecords(params)
  recentPoints.value = response.data?.list || []
  recordTotal.value = Number(response.data?.total || 0)
}

function parseObject(value) { if (typeof value === 'object' && value) return value; try { return JSON.parse(value || '{}') } catch { return {} } }
function formatResult(value) { try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value || '(无输出)' } }
function recordStatusCode(row) { const result = parseObject(row?.result); return Number(result.status_code || result.response?.status_code || 0) }
function formatTime(value) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false }) }
function formatAxisTime(value) { const date = new Date(value); if (Number.isNaN(date.getTime())) return ''; return currentRange.value.ms > 86400000 ? `${date.getMonth() + 1}/${date.getDate()} ${String(date.getHours()).padStart(2, '0')}:00` : `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}` }
function statusGroup(code) { const group = Math.floor(Number(code || 0) / 100); return group >= 2 && group <= 5 ? `${group}xx` : 'other' }

function lineOption({ labels, series, yName, min, max, legend = false, tooltipSuffix = '', zoom = false }) {
  return {
    animationDuration: 450,
    color: series.map(entry => entry.color),
    tooltip: { trigger: 'axis', backgroundColor: 'rgba(20,32,52,.94)', borderWidth: 0, textStyle: { color: '#fff', fontSize: 12 }, valueFormatter: value => value == null ? '无数据' : `${value}${tooltipSuffix}` },
    legend: legend ? { top: 2, right: 8, itemWidth: 18, textStyle: { color: '#64748b' } } : undefined,
    grid: { top: legend ? 42 : 25, right: 22, bottom: zoom ? 48 : 28, left: 52 },
    xAxis: { type: 'category', boundaryGap: false, data: labels, axisLine: { lineStyle: { color: '#dce3ec' } }, axisTick: { show: false }, axisLabel: { color: '#8793a5', hideOverlap: true, fontSize: 10 } },
    yAxis: { type: 'value', name: yName, min, max, splitNumber: 4, nameTextStyle: { color: '#9aa5b4' }, axisLabel: { color: '#8793a5', fontSize: 10 }, splitLine: { lineStyle: { color: '#edf1f6' } } },
    dataZoom: zoom ? [{ type: 'inside', start: 65, end: 100 }, { type: 'slider', height: 16, bottom: 4, borderColor: 'transparent' }] : [],
    series: series.map(entry => ({ name: entry.name, type: 'line', data: entry.data, smooth: .24, connectNulls: false, showSymbol: true, symbolSize: entry.data.length < 20 ? 5 : 3, lineStyle: { width: 2, color: entry.color }, itemStyle: { color: entry.color }, areaStyle: entry.area ? { color: { type: 'linear', x: 0, y: 0, x2: 0, y2: 1, colorStops: [{ offset: 0, color: `${entry.color}38` }, { offset: 1, color: `${entry.color}05` }] } } : undefined })),
  }
}
</script>

<style scoped>
.detail-header{display:flex;min-height:32px;align-items:center;justify-content:space-between;gap:14px}
.detail-page{--border:var(--ops-border);display:flex;flex-direction:column;gap:16px}
.toolbar-actions{display:flex;align-items:center;gap:9px}
.toolbar-actions :deep(.el-select){width:132px}
.toolbar-actions :deep(.el-select:first-child){width:170px}
.chart-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}
.chart-card,.records-card{min-width:0;background:var(--ops-surface);border:1px solid var(--border);border-radius:10px;box-shadow:0 6px 20px rgba(31,50,81,.035)}
.chart-card.full-width{grid-column:1/-1}
.card-heading{display:flex;min-height:61px;align-items:center;justify-content:space-between;padding:0 18px;border-bottom:1px solid var(--ops-border-soft)}
.card-heading>div{display:flex;flex-direction:column}
.card-heading strong{color:#293b53;font-size:13px}
.card-heading span{margin-top:3px;color:var(--ops-muted);font-size:10px}
.card-heading .range-note{margin:0;padding:4px 8px;color:#63758c;background:#f5f8fb;border-radius:5px}
.chart-shell{display:flex;height:290px;align-items:center;justify-content:center;padding:10px}
.chart{width:100%;height:100%}
.wide-chart{height:300px}
.records-card{overflow:hidden}
.records-card :deep(.el-table){border-radius:0 0 10px 10px}
.records-toggle{width:100%;border:0;border-bottom:1px solid transparent;text-align:left;background:transparent;cursor:pointer}
.records-toggle.open{border-bottom-color:var(--ops-border-soft)}
.records-toggle:hover{background:#fafcff}
.records-toggle:focus-visible{outline:2px solid var(--ops-primary);outline-offset:-2px}
.records-meta{display:flex;align-items:center;gap:8px}
.records-meta .range-note{display:inline-flex;align-items:center}
.toggle-icon{margin:0;color:#8391a5;font-size:14px;transition:transform .18s ease}
.records-toggle.open .toggle-icon{transform:rotate(180deg)}
.status-code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:700}
.code-text-2xx{color:#148a67}
.code-text-3xx{color:#216cc9}
.code-text-4xx{color:#c6841e}
.code-text-5xx{color:#d34b4b}
.code-text-other{color:#8995a5}
.record-result{display:block;max-height:180px;overflow:auto;color:#4b5e75;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:11px;line-height:1.5;white-space:pre-wrap;overflow-wrap:anywhere}
.duration-tag{min-width:64px;justify-content:center;border-radius:2px;font-variant-numeric:tabular-nums}
@media(max-width:800px){.chart-grid{grid-template-columns:1fr}.chart-card.full-width{grid-column:auto}}
@media(max-width:560px){.detail-header{align-items:flex-start;flex-wrap:wrap}.toolbar-actions{width:100%;flex-wrap:wrap;justify-content:flex-end}.toolbar-actions :deep(.el-select){width:100%}.chart-shell{height:250px}}
@media(prefers-reduced-motion:reduce){.toggle-icon{transition:none}}
</style>

<template>
  <div class="page-container">
    <div v-if="!source" class="query-context-bar">
      <div v-if="!source" class="datasource-picker">
        <el-select
          v-model="selectedDS"
          class="datasource-select"
          aria-label="指标数据源"
          filterable
          placeholder="选择数据源"
        >
          <el-option v-for="ds in datasources" :key="ds.id" :label="ds.name" :value="ds.id" />
        </el-select>
      </div>

      <el-tooltip content="历史查询" placement="left">
        <el-button class="history-button" aria-label="历史查询" @click="openQueryHistory">
          <svg class="history-icon" viewBox="64 64 896 896" aria-hidden="true">
            <path d="M536.1 273H488c-4.4 0-8 3.6-8 8v275.3c0 2.6 1.2 5 3.3 6.5l165.3 120.7c3.6 2.6 8.6 1.9 11.2-1.7l28.6-39c2.7-3.7 1.9-8.7-1.7-11.2L544.1 528.5V281c0-4.4-3.6-8-8-8zm219.8 75.2l156.8 38.3c5 1.2 9.9-2.6 9.9-7.7l.8-161.5c0-6.7-7.7-10.5-12.9-6.3L752.9 334.1a8 8 0 003 14.1zm167.7 301.1l-56.7-19.5a8 8 0 00-10.1 4.8c-1.9 5.1-3.9 10.1-6 15.1-17.8 42.1-43.3 80-75.9 112.5a353 353 0 01-112.5 75.9 352.18 352.18 0 01-137.7 27.8c-47.8 0-94.1-9.3-137.7-27.8a353 353 0 01-112.5-75.9c-32.5-32.5-58-70.4-75.9-112.5A353.44 353.44 0 01171 512c0-47.8 9.3-94.2 27.8-137.8 17.8-42.1 43.3-80 75.9-112.5a353 353 0 01112.5-75.9C430.6 167.3 477 158 524.8 158s94.1 9.3 137.7 27.8A353 353 0 01775 261.7c10.2 10.3 19.8 21 28.6 32.3l59.8-46.8C784.7 146.6 662.2 81.9 524.6 82 285 82.1 92.6 276.7 95 516.4 97.4 751.9 288.9 942 524.8 942c185.5 0 343.5-117.6 403.7-282.3 1.5-4.2-.7-8.9-4.9-10.4z" />
          </svg>
        </el-button>
      </el-tooltip>
    </div>

    <div class="query-run-row">
      <div class="query-input-row">
        <div class="input-shell">
          <div class="input-leading">
            <el-icon>
              <svg aria-hidden="true" focusable="false" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512">
                <path fill="currentColor" d="M500.3 443.7l-119.7-119.7c27.22-40.41 40.65-90.9 33.46-144.7C401.8 87.79 326.8 13.32 235.2 1.723C99.01-15.51-15.51 99.01 1.724 235.2c11.6 91.64 86.08 166.7 177.6 178.9c53.8 7.189 104.3-6.236 144.7-33.46l119.7 119.7c15.62 15.62 40.95 15.62 56.57 0C515.9 484.7 515.9 459.3 500.3 443.7zM79.1 208c0-70.58 57.42-128 128-128s128 57.42 128 128c0 70.58-57.42 128-128 128S79.1 278.6 79.1 208z" />
              </svg>
            </el-icon>
          </div>
          <textarea
            ref="inputRef"
            v-model="query"
            class="promql-input"
            :placeholder="inputPlaceholder"
            rows="1"
            @keydown="onKeydown"
            @input="onInput"
            spellcheck="false"
          />
          <el-tooltip content="浏览全部指标" placement="top">
            <button type="button" class="metrics-explorer-button" aria-label="浏览全部指标" :disabled="!selectedDS" @click="openMetricsExplorer">
              <svg viewBox="0 0 512 512" xmlns="http://www.w3.org/2000/svg" aria-hidden="true" focusable="false"><path fill="currentColor" d="M512 256C512 397.4 397.4 512 256 512C114.6 512 0 397.4 0 256C0 114.6 114.6 0 256 0C397.4 0 512 114.6 512 256zM266.3 48.25L232.5 73.6C227.2 77.63 224 83.95 224 90.67V99.72C224 106.5 229.5 112 236.3 112C238.7 112 241.1 111.3 243.1 109.9L284.9 82.06C286.9 80.72 289.3 80 291.7 80H292.7C298.9 80 304 85.07 304 91.31C304 94.31 302.8 97.19 300.7 99.31L280.8 119.2C275 124.1 267.9 129.4 260.2 131.9L233.6 140.8C227.9 142.7 224 148.1 224 154.2C224 157.9 222.5 161.5 219.9 164.1L201.9 182.1C195.6 188.4 192 197.1 192 206.1V210.3C192 226.7 205.6 240 221.9 240C232.9 240 243.1 233.8 248 224L252 215.9C254.5 211.1 259.4 208 264.8 208C269.4 208 273.6 210.1 276.3 213.7L292.6 235.5C294.7 238.3 298.1 240 301.7 240C310.1 240 315.6 231.1 311.8 223.6L310.7 221.3C307.1 214.3 310.7 205.8 318.1 203.3L339.3 196.2C346.9 193.7 352 186.6 352 178.6C352 168.3 360.3 160 370.6 160H400C408.8 160 416 167.2 416 176C416 184.8 408.8 192 400 192H379.3C372.1 192 365.1 194.9 360 200L355.3 204.7C353.2 206.8 352 209.7 352 212.7C352 218.9 357.1 224 363.3 224H374.6C380.6 224 386.4 226.4 390.6 230.6L397.2 237.2C398.1 238.1 400 241.4 400 244C400 246.6 398.1 249 397.2 250.8L389.7 258.3C386 261.1 384 266.9 384 272C384 277.1 386 282 389.7 285.7L408 304C418.2 314.2 432.1 320 446.6 320H453.1C460.5 299.8 464 278.3 464 256C464 144.6 376.4 53.64 266.3 48.25V48.25zM438.4 356.1C434.7 353.5 430.2 352 425.4 352C419.4 352 413.6 349.6 409.4 345.4L395.1 331.1C388.3 324.3 377.9 320 367.1 320C357.4 320 347.9 316.5 340.5 310.2L313.1 287.4C302.4 277.5 287.6 271.1 272.3 271.1H251.4C238.7 271.1 226.4 275.7 215.9 282.7L188.5 301C170.7 312.9 160 332.9 160 354.3V357.5C160 374.5 166.7 390.7 178.7 402.7L194.7 418.7C203.2 427.2 214.7 432 226.7 432H248C261.3 432 272 442.7 272 456C272 458.5 272.4 461 273.1 463.3C344.5 457.5 405.6 415.7 438.4 356.1L438.4 356.1zM164.7 100.7L132.7 132.7C126.4 138.9 126.4 149.1 132.7 155.3C138.9 161.6 149.1 161.6 155.3 155.3L187.3 123.3C193.6 117.1 193.6 106.9 187.3 100.7C181.1 94.44 170.9 94.44 164.7 100.7V100.7z" /></svg>
            </button>
          </el-tooltip>
        </div>

        <!-- 自动补全 -->
        <div v-if="showSuggestions" ref="suggestRef" class="autocomplete-dropdown" :style="dropdownStyle">
          <div v-if="suggestLoading" class="suggest-loading">加载中...</div>
          <template v-else>
            <div v-for="(item, idx) in suggestions" :key="item" class="suggest-item"
              :class="{ active: idx === selectedIdx }"
              @mousedown.prevent="selectSuggestion(item)" @mouseenter="selectedIdx = idx">
              <span class="suggest-type-icon">{{ suggestType === 'metric' ? 'M' : suggestType === 'label' ? 'L' : 'V' }}</span>
              <span class="suggest-text">
                <span
                  v-for="(part, partIdx) in highlightedSuggestionParts(item)"
                  :key="`${item}-${partIdx}`"
                  :class="{ matched: part.matched }"
                >{{ part.text }}</span>
              </span>
            </div>
            <div v-if="suggestions.length === 0 && !suggestLoading" class="suggest-empty">无匹配项</div>
          </template>
        </div>
      </div>
      <el-button type="primary" class="execute-button" @click="executeQuery" :loading="loading">Execute</el-button>
    </div>

    <div class="result-panel">
      <div class="result-tabs">
        <button class="tab" :class="{ active: activeTab === 'table' }" type="button" @click="switchToTable()">
          <span>Table</span>
        </button>
        <button class="tab" :class="{ active: activeTab === 'graph' }" type="button" @click="switchToGraph()">
          <span>Graph</span>
        </button>
        <div class="tab-meta">
          <span v-if="result" class="result-count">{{ result.data?.result?.length || 0 }} 条结果</span>
          <span v-if="execTime" class="exec-time">{{ execTime }}</span>
        </div>
      </div>

      <!-- Table -->
      <div v-show="activeTab === 'table'" class="result-content table-content">
        <div class="panel-toolbar">
          <div class="eval-control">
            <button type="button" class="eval-arrow" title="减少 30 分钟" aria-label="减少 30 分钟" @click="shiftEvalTime(-1)"><svg viewBox="0 0 320 512" aria-hidden="true" focusable="false"><path fill="currentColor" d="M224 480c-8.188 0-16.38-3.125-22.62-9.375l-192-192c-12.5-12.5-12.5-32.75 0-45.25l192-192c12.5-12.5 32.75-12.5 45.25 0s12.5 32.75 0 45.25L77.25 256l169.4 169.4c12.5 12.5 12.5 32.75 0 45.25C240.4 476.9 232.2 480 224 480z" /></svg></button>
            <el-date-picker
              v-model="evalTime"
              type="datetime"
              class="eval-picker"
              placeholder="Evaluation time"
              aria-label="Evaluation time"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DD HH:mm:ss"
              size="small"
              editable
              clearable
              @change="onTimeChange"
            />
            <button type="button" class="eval-arrow" title="增加 30 分钟" aria-label="增加 30 分钟" @click="shiftEvalTime(1)"><svg viewBox="0 0 320 512" aria-hidden="true" focusable="false"><path fill="currentColor" d="M96 480c-8.188 0-16.38-3.125-22.62-9.375c-12.5-12.5-12.5-32.75 0-45.25L242.8 256L73.38 86.63c-12.5-12.5-12.5-32.75 0-45.25s32.75-12.5 45.25 0l192 192c12.5 12.5 12.5 32.75 0 45.25l-192 192C112.4 476.9 104.2 480 96 480z" /></svg></button>
          </div>
        </div>
        <div v-if="error" class="error-box"><el-icon color="#F56C6C"><WarningFilled /></el-icon>{{ error }}</div>
        <el-table v-else-if="result" :data="tableRows" class="query-result-table" stripe size="small" max-height="55vh" style="width:100%">
          <el-table-column label="数据源" width="180" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="result-source">{{ row.datasourceName }}</span>
            </template>
          </el-table-column>
          <el-table-column label="标签" min-width="360" show-overflow-tooltip>
            <template #default="{ row }"><span class="metric-cell">{{ row.element }}</span></template>
          </el-table-column>
          <el-table-column label="值" width="180" align="left" header-align="left" show-overflow-tooltip>
            <template #default="{ row }"><span class="value-cell">{{ row.displayValue }}</span></template>
          </el-table-column>
          <template #empty><TableEmpty /></template>
        </el-table>
        <div v-else class="query-alert" role="status">No data queried yet</div>
      </div>

      <!-- Graph -->
      <div v-show="activeTab === 'graph'" class="result-content">
        <form class="graph-controls" @submit.prevent="executeQuery">
          <div class="graph-input-group range-input">
            <button type="button" title="Decrease range" aria-label="Decrease range" @click="shiftGraphRange(-1)"><svg viewBox="0 0 448 512" aria-hidden="true"><path d="M400 224H48a32 32 0 000 64h352a32 32 0 000-64z" /></svg></button>
            <input v-model="graphRange" aria-label="Range" type="text" @change="onGraphRangeChange">
            <button type="button" title="Increase range" aria-label="Increase range" @click="shiftGraphRange(1)"><svg viewBox="0 0 448 512" aria-hidden="true"><path d="M432 224H256V48a32 32 0 00-64 0v176H16a32 32 0 000 64h176v176a32 32 0 0064 0V288h176a32 32 0 000-64z" /></svg></button>
          </div>
          <div class="graph-input-group time-input">
            <button type="button" title="Decrease time" aria-label="Decrease time" @click="shiftGraphTime(-1)"><svg viewBox="0 0 320 512" aria-hidden="true"><path d="M224 480a32 32 0 01-23-9L9 279a32 32 0 010-46L201 41a32 32 0 0146 46L77 256l170 169a32 32 0 01-23 55z" /></svg></button>
            <el-date-picker
              v-model="graphEndTime"
              class="graph-end-picker"
              type="datetime"
              placeholder="End time"
              aria-label="End time"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DD HH:mm:ss"
              size="small"
              editable
              clearable
              @change="onGraphParamChange"
            />
            <button type="button" title="Increase time" aria-label="Increase time" @click="shiftGraphTime(1)"><svg viewBox="0 0 320 512" aria-hidden="true"><path d="M96 480a32 32 0 01-23-55l170-169L73 87a32 32 0 0146-46l192 192a32 32 0 010 46L119 471a32 32 0 01-23 9z" /></svg></button>
          </div>
          <input v-model="graphStep" class="resolution-input" placeholder="Res. (s)" aria-label="Resolution in seconds" inputmode="decimal" type="text" @change="onGraphParamChange">
          <div class="graph-input-group stacked-input" role="group" aria-label="Graph style">
            <button type="button" title="Show unstacked line graph" aria-label="Show unstacked line graph" :class="{ active: !graphStacked }" :aria-pressed="!graphStacked" @click="graphStacked = false"><svg viewBox="0 0 512 512" aria-hidden="true"><path d="M64 400a16 16 0 0016 16h400a32 32 0 010 64H80a80 80 0 01-80-80V64a32 32 0 0164 0zm279-121a32 32 0 01-46 0l-57-58-89 90a32 32 0 01-46-46l112-112a32 32 0 0146 0l57 58 105-106a32 32 0 0146 46z" /></svg></button>
            <button type="button" title="Show stacked graph" aria-label="Show stacked graph" :class="{ active: graphStacked }" :aria-pressed="graphStacked" @click="graphStacked = true"><svg viewBox="0 0 512 512" aria-hidden="true"><path d="M64 400a16 16 0 0016 16h400a32 32 0 010 64H80a80 80 0 01-80-80V64a32 32 0 0164 0zm64-80v-84q0-12 8-22l79-90q24-24 48 1l40 47q17 20 36 1l21-21q24-24 47 2l65 77q8 9 8 21v68q0 32-32 32H160q-32 0-32-32z" /></svg></button>
          </div>
        </form>
        <div v-if="error" class="error-box"><el-icon color="#F56C6C"><WarningFilled /></el-icon>{{ error }}</div>
        <div v-else-if="chartSeries.length === 0" class="empty-chart">
          当前结果无可渲染的图表数据（请切换到范围查询模式）
        </div>
        <template v-else>
          <v-chart :option="chartOption" :autoresize="true" style="height:380px" />
          <div class="metric-legend" aria-label="图表指标">
            <div class="metric-legend-header" aria-hidden="true">
              <span>数据源</span>
              <span>标签</span>
            </div>
            <button
              v-for="(series, index) in chartSeries"
              :key="`${series.name}-${index}`"
              type="button"
              class="metric-legend-item"
              :class="{ active: hasSeriesFilter && isMetricSelected(index), muted: hasSeriesFilter && !isMetricSelected(index) }"
              :aria-pressed="isMetricSelected(index)"
              title="点击仅显示此指标，Shift + 点击可多选"
              @click="toggleMetric(index, $event)"
            >
              <span class="metric-legend-line" :style="{ '--series-color': chartColors[index % chartColors.length] }" aria-hidden="true"></span>
              <span class="metric-legend-source" :title="series.datasourceName">{{ series.datasourceName }}</span>
              <span class="metric-legend-name">{{ series.metricName }}</span>
            </button>
          </div>
        </template>
      </div>
    </div>

    <el-dialog v-model="metricsExplorerVisible" title="Metrics Explorer" width="min(620px, 94vw)" class="metrics-explorer-dialog" append-to-body @closed="closeMetricsExplorer" @opened="metricsSearchRef?.focus()">
      <el-input ref="metricsSearchRef" v-model="metricsSearch" placeholder="Search" aria-label="搜索指标名称" clearable />
      <div class="metrics-explorer-meta"><span>{{ filteredMetrics.length }} / {{ explorerMetrics.length }} 个指标</span><el-button text size="small" :loading="metricsLoading" @click="loadExplorerMetrics">刷新</el-button></div>
      <el-alert v-if="metricsError" :title="metricsError" type="error" :closable="false" show-icon />
      <div v-loading="metricsLoading" class="metrics-explorer-list" aria-label="指标列表">
        <button v-for="metric in visibleMetrics" :key="metric" type="button" class="metrics-explorer-item" @click="chooseExplorerMetric(metric)">{{ metric }}</button>
        <el-empty v-if="!metricsLoading && !filteredMetrics.length && !metricsError" :description="metricsSearch ? '没有匹配的指标' : '当前数据源没有可用指标'" :image-size="64" />
      </div>
      <el-pagination v-if="filteredMetrics.length > 100" v-model:current-page="metricsPage" :page-size="100" :total="filteredMetrics.length" layout="prev, pager, next" small />
    </el-dialog>

    <QueryHistoryDrawer ref="historyDrawer" @select="restoreQuery" />
  </div>
</template>

<script setup>
import { useThemeColors } from '../../theme/useThemeColors'
const themeColors = useThemeColors()
import QueryHistoryDrawer from '../../components/explore/QueryHistoryDrawer.vue'
import { ref, reactive, onMounted, onBeforeUnmount, computed, nextTick, watch } from 'vue'
import { promInstantQuery, promRangeQuery, getPromDatasources, promMetadata } from '../../api/prometheus'
import { ElMessage } from 'element-plus'
import { WarningFilled } from '@element-plus/icons-vue'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, DataZoomComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

use([LineChart, GridComponent, TooltipComponent, DataZoomComponent, CanvasRenderer])

const props = defineProps({ source: { type: Object, default: null } })
const datasources = ref(props.source ? [props.source] : [])
const selectedDS = ref(props.source?.id ?? null)
const query = ref('')
const loading = ref(false)
const result = ref(null)
const error = ref('')
const activeTab = ref('table')
const execTime = ref('')
const evalTime = ref('')
const inputRef = ref(null)
const resultMode = ref('')
const metricsExplorerVisible = ref(false), metricsSearch = ref(''), metricsSearchRef = ref(null)
const explorerMetrics = ref([]), metricsLoading = ref(false), metricsError = ref(''), metricsPage = ref(1)
const filteredMetrics = computed(() => {
  const term = metricsSearch.value.trim().toLowerCase()
  return explorerMetrics.value.filter(metric => metric.toLowerCase().includes(term))
})
const visibleMetrics = computed(() => filteredMetrics.value.slice((metricsPage.value - 1) * 100, metricsPage.value * 100))
watch(metricsSearch, () => { metricsPage.value = 1 })
let metricsController, metricsRequestID = 0
function closeMetricsExplorer() { metricsRequestID++; metricsController?.abort(); metricsLoading.value = false }
async function openMetricsExplorer() {
  metricsExplorerVisible.value = true
  metricsSearch.value = ''
  showSuggestions.value = false
  await loadExplorerMetrics()
}
async function loadExplorerMetrics() {
  metricsController?.abort()
  const current = ++metricsRequestID
  metricsController = new AbortController()
  metricsLoading.value = true; metricsError.value = ''; explorerMetrics.value = []; metricsPage.value = 1
  if (!selectedDS.value) { metricsLoading.value = false; metricsError.value = '请先选择指标数据源'; return }
  try {
    const response = await promMetadata({ datasource_id: selectedDS.value, target: 'metrics' }, { signal: metricsController.signal, timeout: 35000 })
    if (current !== metricsRequestID) return
    if (!Array.isArray(response.data)) throw new Error('指标接口返回格式不正确')
    explorerMetrics.value = [...new Set(response.data.filter(item => typeof item === 'string'))].sort((a, b) => a.localeCompare(b))
  } catch (e) {
    if (current === metricsRequestID) metricsError.value = `指标列表加载失败：${e?.response?.data?.message || e.message || '请求失败'}`
  } finally {
    if (current === metricsRequestID) metricsLoading.value = false
  }
}
function chooseExplorerMetric(metric) {
  query.value = metric
  showSuggestions.value = false
  metricsExplorerVisible.value = false
  nextTick(() => { inputRef.value?.focus(); inputRef.value?.setSelectionRange(metric.length, metric.length) })
}
watch(selectedDS, () => { closeMetricsExplorer(); explorerMetrics.value = []; if (metricsExplorerVisible.value) loadExplorerMetrics() })
onBeforeUnmount(closeMetricsExplorer)

const historyDrawer = ref(null)
function openQueryHistory() { return historyDrawer.value?.openQueryHistory() }
defineExpose({ openQueryHistory })
function restoreQuery(item) {
  query.value = item.query
  nextTick(() => inputRef.value?.focus())
}

const rangeOptions = ['1s', '10s', '1m', '5m', '15m', '30m', '1h', '2h', '6h', '12h', '1d', '2d', '7d', '30d', '90d', '1y']
const graphRange = ref('1h')
const graphEndTime = ref('')
const graphStep = ref('')
const graphStacked = ref(false)
const maxPointsPerSeries = 11000

const inputPlaceholder = '输入 PromQL 表达式'

onMounted(async () => {
  try {
    const res = props.source ? { data: [props.source] } : await getPromDatasources()
    datasources.value = res.data || []
    if (datasources.value.length > 0) selectedDS.value = datasources.value.find(item => item.id === props.source?.id)?.id || datasources.value[0].id
  } catch {}
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => document.removeEventListener('click', handleClickOutside))

function formatDateTime(d) {
  const pad = n => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function toPromTimestamp(value) {
  if (!value) return undefined
  const date = value instanceof Date ? value : new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

// ── 自动补全 ──

const showSuggestions = ref(false)
const suggestions = ref([])
const selectedIdx = ref(0)
const suggestLoading = ref(false)
const suggestType = ref('metric')
const suggestToken = ref('')
const replaceRange = ref({ start: 0, end: 0 })
const dropdownStyle = ref({ top: '0px', left: '0px' })
let suggestTimer = null

function onInput() {
  const el = inputRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 150) + 'px'
  clearTimeout(suggestTimer)
  const text = query.value
  const pos = el.selectionStart
  suggestTimer = setTimeout(() => triggerSuggest(text, pos, el), 200)
}

function triggerSuggest(text, pos, el) {
  const { token, start, end, type, labelName } = parseTokenAt(text, pos)
  if (!token || !selectedDS.value) { showSuggestions.value = false; return }
  suggestType.value = type
  suggestToken.value = token
  replaceRange.value = { start, end }
  const posPx = cursorPixelPos(el, start)
  dropdownStyle.value = { top: (posPx.top + 22) + 'px', left: (posPx.left + 4) + 'px' }
  suggestLoading.value = true; showSuggestions.value = true; selectedIdx.value = 0; suggestions.value = []
  promMetadata({ datasource_id: selectedDS.value, target: type === 'label' ? 'labels' : type === 'value' ? 'label_values' : 'metrics', ...(type === 'value' ? { label: labelName } : {}) }).then(response => {
    suggestions.value = fuzzySuggestions(response.data || [], token).slice(0, 30)
  }).catch(() => { showSuggestions.value = false }).finally(() => { suggestLoading.value = false })
}

function fuzzySuggestions(values, token) {
  const keyword = String(token || '').trim()
  if (!keyword) return []
  return values
    .map(value => ({ value: String(value), score: suggestionScore(String(value), keyword) }))
    .filter(item => item.score < Number.POSITIVE_INFINITY)
    .sort((a, b) => a.score - b.score || a.value.length - b.value.length || a.value.localeCompare(b.value))
    .map(item => item.value)
}

function suggestionScore(value, token) {
  const lowerValue = value.toLowerCase()
  const lowerToken = token.toLowerCase()
  const includesAt = lowerValue.indexOf(lowerToken)
  if (includesAt >= 0) return includesAt * 2 + Math.max(0, value.length - token.length) * 0.01

  const positions = fuzzyMatchPositions(value, token)
  if (positions.length === token.length) {
    const span = positions[positions.length - 1] - positions[0]
    const separators = positions.filter(pos => '_:-.'.includes(value[pos - 1] || '')).length
    return 100 + span - separators * 4 + Math.max(0, value.length - token.length) * 0.01
  }

  const tokenParts = splitSuggestToken(token)
  const matchedParts = tokenParts.filter(part => lowerValue.includes(part.toLowerCase()))
  if (matchedParts.length > 0) {
    const firstPartAt = lowerValue.indexOf(matchedParts[0].toLowerCase())
    return 200 + firstPartAt + (tokenParts.length - matchedParts.length) * 20
  }

  return Number.POSITIVE_INFINITY
}

function splitSuggestToken(token) {
  const normalized = String(token || '').replace(/[_:.-]+/g, ' ')
  const explicitParts = normalized.split(/\s+/).filter(part => part.length >= 2)
  if (explicitParts.length > 1) return explicitParts

  const commonParts = ['cpu', 'memory', 'mem', 'total', 'seconds', 'bytes', 'http', 'request', 'requests', 'node', 'container', 'process', 'rate']
  const lowerToken = token.toLowerCase()
  const parts = commonParts.filter(part => lowerToken.includes(part))
  return parts.length > 0 ? parts : [token]
}

function fuzzyMatchPositions(value, token) {
  const lowerValue = value.toLowerCase()
  const lowerToken = token.toLowerCase()
  const positions = []
  let searchFrom = 0
  for (const char of lowerToken) {
    const pos = lowerValue.indexOf(char, searchFrom)
    if (pos < 0) return []
    positions.push(pos)
    searchFrom = pos + 1
  }
  return positions
}

function highlightedSuggestionParts(value) {
  const token = suggestToken.value
  if (!token) return [{ text: value, matched: false }]

  const lowerValue = value.toLowerCase()
  const lowerToken = token.toLowerCase()
  const includesAt = lowerValue.indexOf(lowerToken)
  if (includesAt >= 0) {
    return splitByMatchedPositions(value, Array.from({ length: token.length }, (_, idx) => includesAt + idx))
  }

  const positions = fuzzyMatchPositions(value, token)
  if (positions.length === token.length) {
    return splitByMatchedPositions(value, positions)
  }

  const matchedPositions = new Set()
  for (const part of splitSuggestToken(token)) {
    const start = lowerValue.indexOf(part.toLowerCase())
    if (start >= 0) {
      for (let idx = start; idx < start + part.length; idx += 1) matchedPositions.add(idx)
    }
  }
  return splitByMatchedPositions(value, [...matchedPositions].sort((a, b) => a - b))
}

function splitByMatchedPositions(value, positions) {
  if (!positions.length) return [{ text: value, matched: false }]
  const matched = new Set(positions)
  const parts = []
  let buffer = ''
  let currentMatched = matched.has(0)
  for (let idx = 0; idx < value.length; idx += 1) {
    const isMatched = matched.has(idx)
    if (idx > 0 && isMatched !== currentMatched) {
      parts.push({ text: buffer, matched: currentMatched })
      buffer = ''
      currentMatched = isMatched
    }
    buffer += value[idx]
  }
  if (buffer) parts.push({ text: buffer, matched: currentMatched })
  return parts
}

function parseTokenAt(text, pos) {
  const before = text.slice(0, pos)
  let braceIdx = -1, braceDepth = 0
  for (let i = before.length - 1; i >= 0; i--) {
    if (before[i] === '}') braceDepth++
    else if (before[i] === '{') { if (braceDepth === 0) { braceIdx = i; break } braceDepth-- }
  }
  if (braceIdx >= 0) {
    const inside = before.slice(braceIdx + 1)
    const eqMatch = inside.match(/(\w+)\s*=\s*(~?\s*")([^"]*)$/)
    if (eqMatch) return { token: eqMatch[3] || '', start: braceIdx + 1 + eqMatch.index + eqMatch[1].length + eqMatch[2].length, end: pos, type: 'value', labelName: eqMatch[1] }
    const commaMatch = inside.match(/(?:,\s*|^\s*)(\w*)$/)
    if (commaMatch) return { token: commaMatch[1], start: braceIdx + 1 + commaMatch.index + (commaMatch[0].length - commaMatch[1].length), end: pos, type: 'label' }
  }
  const tokenMatch = before.match(/([a-zA-Z_:][a-zA-Z0-9_:]*)$/)
  if (tokenMatch) return { token: tokenMatch[1], start: pos - tokenMatch[1].length, end: pos, type: 'metric' }
  return { token: '', start: 0, end: 0, type: 'metric', labelName: '' }
}

function cursorPixelPos(textarea, charIdx) {
  const style = getComputedStyle(textarea)
  const mirror = document.createElement('div')
  Object.assign(mirror.style, { position:'absolute', visibility:'hidden', whiteSpace:'pre-wrap', wordWrap:'break-word', overflowWrap:'break-word', font:style.font, fontFamily:style.fontFamily, fontSize:style.fontSize, lineHeight:style.lineHeight, padding:style.padding, border:style.border, width:style.width, boxSizing:style.boxSizing })
  document.body.appendChild(mirror)
  const before = textarea.value.slice(0, charIdx)
  mirror.textContent = before
  const marker = document.createElement('span'); marker.textContent = '​'; mirror.appendChild(marker)
  const taRect = textarea.getBoundingClientRect()
  const lineH = parseFloat(style.lineHeight) || 20
  const lines = before.split('\n')
  document.body.removeChild(mirror)
  return { top: taRect.top - textarea.scrollTop + (lines.length - 1) * lineH + lineH, left: taRect.left - textarea.scrollLeft + (marker.offsetLeft || lines[lines.length-1].length * 8) }
}

function onKeydown(e) {
  if (!showSuggestions.value || suggestions.value.length === 0) {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); executeQuery() }
    return
  }
  switch (e.key) {
    case 'ArrowDown': e.preventDefault(); selectedIdx.value = (selectedIdx.value + 1) % suggestions.value.length; break
    case 'ArrowUp': e.preventDefault(); selectedIdx.value = (selectedIdx.value - 1 + suggestions.value.length) % suggestions.value.length; break
    case 'Enter': case 'Tab': e.preventDefault(); selectSuggestion(suggestions.value[selectedIdx.value]); break
    case 'Escape': showSuggestions.value = false; break
  }
}

function selectSuggestion(item) {
  const text = query.value; const start = replaceRange.value.start; const end = replaceRange.value.end
  query.value = text.slice(0, start) + item + text.slice(end)
  showSuggestions.value = false
  nextTick(() => { const el = inputRef.value; if (el) { const np = start + item.length; el.setSelectionRange(np, np); el.focus(); onInput() } })
}

function handleClickOutside(e) {
  const wrap = document.querySelector('.query-input-row')
  if (wrap && !wrap.contains(e.target)) showSuggestions.value = false
}

// ── 查询执行 ──

let execTimer = null

function validEvaluationTime() {
  if (!String(evalTime.value || '').trim() || toPromTimestamp(evalTime.value)) return true
  ElMessage.warning('请输入有效时间，例如 2026-10-24 14:30:00；留空使用当前时间')
  return false
}

function onTimeChange() {
  clearTimeout(execTimer)
  if (!validEvaluationTime()) return
  execTimer = setTimeout(() => executeQuery(), 100)
}

function shiftEvalTime(direction) {
  if (!validEvaluationTime()) return
  const base = evalTime.value ? new Date(evalTime.value) : new Date()
  if (Number.isNaN(base.getTime())) return
  base.setTime(base.getTime() + direction * 30 * 60 * 1000)
  evalTime.value = formatDateTime(base).replace('T', ' ')
  onTimeChange()
}

function onGraphParamChange() {
  if (!validGraphParams()) return
  clearTimeout(execTimer)
  execTimer = setTimeout(() => switchToGraph(true), 100)
}

function onGraphRangeChange() {
  if (!validGraphParams()) return
  const adjustedStep = ensureValidGraphStep()
  if (adjustedStep) ElMessage.info(`${graphRange.value} 查询范围已自动将 Step 调整为 ${adjustedStep}`)
  onGraphParamChange()
}

function switchToTable(forceExec = false) {
  activeTab.value = 'table'
  if (forceExec || resultMode.value !== 'table') executeInstantQuery()
}

function switchToGraph(forceExec = false) {
  activeTab.value = 'graph'
  if (forceExec || resultMode.value !== 'graph') executeRangeQuery()
}

async function executeQuery() {
  if (!query.value.trim() || !selectedDS.value) return
  if (activeTab.value === 'graph') {
    await executeRangeQuery()
  } else {
    await executeInstantQuery()
  }
}

async function executeInstantQuery() {
  if (!validEvaluationTime()) return
  if (!query.value.trim() || !selectedDS.value) return
  showSuggestions.value = false
  loading.value = true; error.value = ''; result.value = null
  const startTime = performance.now()
  try {
    result.value = await querySelectedDatasource(dsId => promInstantQuery({ datasource_id: dsId, query: query.value, time: toPromTimestamp(evalTime.value) }))
    resultMode.value = 'table'
  } catch (e) { error.value = e?.response?.data?.message || e?.message || '请求失败' }
  finally {
    execTime.value = ((performance.now() - startTime) / 1000).toFixed(2) + 's'
    loading.value = false
    if (result.value?.status === 'error') { error.value = result.value.error || '指标数据源返回错误'; result.value = null }
  }
}

async function executeRangeQuery() {
  if (!validGraphParams()) return
  if (!query.value.trim() || !selectedDS.value) return
  loading.value = true; error.value = ''; result.value = null
  const startTime = performance.now()
  try {
    ensureValidGraphStep()
    const end = graphEndTime.value || formatDateTime(new Date())
    const start = calcRangeStart(end, graphRange.value)
    result.value = await querySelectedDatasource(dsId => promRangeQuery({
      datasource_id: dsId,
      query: query.value,
      start: toPromTimestamp(start),
      end: toPromTimestamp(end),
      step: `${effectiveGraphStep()}s`,
    }))
    resultMode.value = 'graph'
    selectedSeriesIndexes.value = []
  } catch (e) { error.value = e?.response?.data?.message || e?.message || '请求失败' }
  finally {
    execTime.value = ((performance.now() - startTime) / 1000).toFixed(2) + 's'
    loading.value = false
    if (result.value?.status === 'error') { error.value = result.value.error || '指标数据源返回错误'; result.value = null }
  }
}

function rangeSeconds(range) {
  const match = /^(\d+(?:\.\d+)?)(s|m|h|d|w|y)$/.exec(range.trim())
  return match ? Number(match[1]) * ({ s: 1, m: 60, h: 3600, d: 86400, w: 604800, y: 31536000 }[match[2]]) : NaN
}

function validGraphParams() {
  if (!(rangeSeconds(graphRange.value) > 0)) { ElMessage.warning('请输入有效范围，例如 30m、1h、7d'); return false }
  if (graphEndTime.value && !toPromTimestamp(graphEndTime.value)) { ElMessage.warning('请输入有效的结束时间，例如 2026-10-24 12:00:00'); return false }
  if (graphStep.value.trim() && (!Number.isFinite(Number(graphStep.value)) || Number(graphStep.value) <= 0)) { ElMessage.warning('分辨率必须为正数，单位为秒'); return false }
  return true
}

function effectiveGraphStep() {
  return Number(graphStep.value) || Math.max(1, Math.ceil(rangeSeconds(graphRange.value) / 250))
}

function ensureValidGraphStep() {
  const minimum = Math.ceil(rangeSeconds(graphRange.value) / (maxPointsPerSeries - 1))
  if (effectiveGraphStep() >= minimum) return ''
  graphStep.value = String(minimum)
  return `${minimum}s`
}

function shiftGraphRange(direction) {
  const current = rangeSeconds(graphRange.value)
  if (!Number.isFinite(current)) { validGraphParams(); return }
  const options = direction > 0 ? rangeOptions : [...rangeOptions].reverse()
  graphRange.value = options.find(value => direction > 0 ? rangeSeconds(value) > current : rangeSeconds(value) < current) || graphRange.value
  onGraphRangeChange()
}

function shiftGraphTime(direction) {
  if (!validGraphParams()) return
  const base = graphEndTime.value ? new Date(graphEndTime.value) : new Date()
  graphEndTime.value = formatDateTime(new Date(base.getTime() + direction * rangeSeconds(graphRange.value) * 500)).replace('T', ' ')
  onGraphParamChange()
}

async function querySelectedDatasource(queryFn) {
  const datasourceID = selectedDS.value
  const datasourceName = datasources.value.find(item => item.id === datasourceID)?.name || `数据源 ${datasourceID}`
  const response = await queryFn(datasourceID)
  return {
    ...response,
    data: {
      ...response.data,
      result: (response.data?.result || []).map(series => ({ ...series, datasource_id: datasourceID, datasource_name: datasourceName })),
    },
  }
}

function calcRangeStart(endStr, rangeStr) {
  const end = new Date(endStr)
  return formatDateTime(new Date(end.getTime() - rangeSeconds(rangeStr) * 1000))
}

// ── 表格 ──

const tableRows = computed(() => {
  if (!result.value?.data?.result) return []
  return result.value.data.result.map(r => {
    const metric = { ...r.metric }; const name = metric.__name__ || ''; delete metric.__name__
    const labelParts = Object.entries(metric).map(([k,v]) => `${k}="${v}"`)
    const element = labelParts.length > 0 ? `${name}{${labelParts.join(',')}}` : name
    const val = r.value?.[1]; const values = r.values
    return { datasourceName: r.datasource_name, element: values ? `${element}  (${values.length} 个点)` : element, value: val, displayValue: formatValue(val) }
  })
})

function formatValue(v) {
  if (v === undefined || v === null) return '-'
  const num = Number(v); return isNaN(num) ? String(v) : num
}

function formatAxisValue(value) {
  const number = Number(value)
  if (!Number.isFinite(number)) return '-'
  const units = ['', 'K', 'M', 'B', 'T']
  let scaled = number
  let unit = 0
  while (Math.abs(scaled) >= 999.995 && unit < units.length - 1) {
    scaled /= 1000
    unit++
  }
  return `${(Math.abs(scaled) < 0.005 ? 0 : scaled).toFixed(2)}${units[unit]}`
}

// ── 图表 ──

const selectedSeriesIndexes = ref([])
const chartColors = ['#5470c6', '#91cc75', '#fac858', '#ee6666', '#73c0de', '#3ba272', '#fc8452', '#9a60b4', '#ea7ccc']

const chartSeries = computed(() => {
  if (!result.value?.data?.result) return []
  return result.value.data.result.map((r, index) => {
    const metric = { ...r.metric }; const name = metric.__name__ || Object.values(metric).join(',') || 'value'
    const labelStr = Object.entries(metric).filter(([k]) => k !== '__name__').map(([k,v]) => `${k}="${v}"`).join(',')
    const metricName = labelStr ? `${name}{${labelStr}}` : name
    const data = result.value.data.resultType === 'vector'
      ? [[Number(r.value[0]) * 1000, Number(r.value[1])]]
      : (r.values || []).map(([ts, val]) => [Number(ts) * 1000, Number(val)])
    const color = chartColors[index % chartColors.length]
    return {
      name: metricName,
      datasourceName: r.datasource_name,
      metricName,
      type: 'line',
      stack: graphStacked.value ? 'total' : undefined,
      areaStyle: graphStacked.value ? { opacity: 0.6 } : undefined,
      symbol: 'none',
      data,
      lineStyle: { color },
      itemStyle: { color },
    }
  })
})

const hasSeriesFilter = computed(() => selectedSeriesIndexes.value.length > 0)
const visibleChartSeries = computed(() => {
  if (!hasSeriesFilter.value) return chartSeries.value
  return chartSeries.value.filter((_, index) => selectedSeriesIndexes.value.includes(index))
})

function isMetricSelected(index) {
  return !hasSeriesFilter.value || selectedSeriesIndexes.value.includes(index)
}

function toggleMetric(index, event) {
  const selected = selectedSeriesIndexes.value

  if (!event.shiftKey) {
    selectedSeriesIndexes.value = selected.length === 1 && selected[0] === index ? [] : [index]
    return
  }

  if (selected.length === 0) {
    selectedSeriesIndexes.value = [index]
    return
  }

  if (selected.includes(index)) {
    if (selected.length > 1) selectedSeriesIndexes.value = selected.filter(item => item !== index)
    return
  }

  selectedSeriesIndexes.value = [...selected, index].sort((a, b) => a - b)
}

const chartOption = computed(() => ({
  color: chartColors,
  tooltip: { backgroundColor: themeColors.value.surface, borderColor: themeColors.value.border, textStyle: { color: themeColors.value.text }, trigger: 'axis', formatter(p) { let h = `<small>${new Date(p[0].data[0]).toLocaleString()}</small><br/>`; p.forEach(i => h += `${i.marker} ${i.seriesName}: <b>${i.data[1]}</b><br/>`); return h }},
  grid: { left: 60, right: 20, top: 10, bottom: 32 },
  xAxis: { type: 'time', axisLabel: { color: themeColors.value.muted }, axisLine: { lineStyle: { color: themeColors.value.border } } },
  yAxis: { type: 'value', axisLabel: { color: themeColors.value.muted, formatter: formatAxisValue }, splitLine: { lineStyle: { color: themeColors.value['border-soft'] } } },
  dataZoom: [{ type: 'inside' }],
  series: visibleChartSeries.value,
}))
</script>

<style scoped>
.metrics-explorer-button { display: flex; align-items: center; justify-content: center; flex: 0 0 36px; align-self: stretch; min-height: 36px; border: 0; border-left: 1px solid var(--ops-border-soft); background: transparent; color: var(--ops-muted); cursor: pointer; padding: 8px; }
.metrics-explorer-button:hover, .metrics-explorer-button:focus-visible { color: var(--ops-primary); background: var(--ops-surface-soft); }
.metrics-explorer-button:disabled { opacity: .45; cursor: not-allowed; }
.metrics-explorer-button svg { width: 17px; height: 17px; fill: currentColor; }
.metrics-explorer-meta { display: flex; justify-content: space-between; align-items: center; margin: 10px 0; font-size: 12px; color: var(--ops-muted); }
.metrics-explorer-list { height: min(440px, 55vh); overflow: auto; margin: 10px 0; }
.metrics-explorer-item { display: block; width: 100%; padding: 7px 10px; border: 0; border-radius: 3px; background: transparent; color: var(--ops-text); text-align: left; font: 13px/1.5 Consolas, monospace; overflow-wrap: anywhere; cursor: pointer; }
.metrics-explorer-item:hover, .metrics-explorer-item:focus-visible { background: var(--ops-surface-soft); color: var(--ops-primary); }

.page-container { display: flex; flex-direction: column; gap: 10px; }
.query-context-bar {
  display: flex;
  min-height: 36px;
  align-items: center;
  gap: 8px;
  margin-bottom: 6px;
}
.datasource-picker {
  display: flex;
  align-items: center;
  min-height: 36px;
  flex: 0 1 260px;
}
.datasource-select {
  width: 100%;
  min-width: 0;
}
.datasource-select :deep(.el-select__wrapper) {
  min-height: 36px;
  padding: 3px 11px;
  background: var(--ops-surface);
  border-radius: 4px;
  box-shadow: 0 0 0 1px var(--ops-border) inset;
  transition: box-shadow .16s ease;
}
.datasource-select :deep(.el-select__wrapper.is-focused) {
  box-shadow: 0 0 0 1px var(--ops-primary) inset;
}
.datasource-select :deep(.el-select__tags-text) {
  max-width: 190px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.history-button {
  width: 36px;
  height: 36px;
  margin-left: auto;
  padding: 0;
  color: var(--ops-muted);
  background: var(--ops-surface);
  border-color: var(--ops-border);
  border-radius: 6px;
}
.history-button:hover,
.history-button:focus-visible {
  color: var(--ops-primary);
  border-color: #a8cdf7;
  background: var(--ops-primary-soft);
}
.history-icon {
  width: 16px;
  height: 16px;
  fill: currentColor;
}
.toolbar-label { font-size: 13px; font-weight: 500; color: var(--ops-muted); white-space: nowrap; }

/* ── 查询输入行 ── */
.query-run-row { display: flex; align-items: flex-start; gap: 8px; }
.query-input-row {
  position: relative;
  flex: 1;
  min-width: 0;
}
.input-shell {
  display: flex;
  align-items: stretch;
  min-height: 40px;
  overflow: hidden;
  background: var(--ops-surface);
  border: 1px solid var(--ops-border);
  border-radius: 6px;
  transition: border-color .16s ease, box-shadow .16s ease;
}
.input-shell:focus-within {
  border-color: var(--ops-primary);
  box-shadow: 0 0 0 2px rgba(64, 158, 255, .12);
}
.input-leading {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 42px;
  flex: 0 0 42px;
  color: var(--ops-muted);
  background: var(--ops-surface-soft);
  border-right: 1px solid var(--ops-border-soft);
}
.promql-input {
  flex: 1;
  min-width: 0;
  border: none;
  outline: none;
  padding: 9px 12px;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 14px;
  line-height: 1.5;
  color: var(--ops-text);
  resize: none;
  overflow: hidden;
  box-sizing: border-box;
  background: var(--ops-surface);
}
.promql-input::placeholder { color: var(--ops-subtle); font-family: inherit; }
.execute-button { flex: 0 0 auto; min-height: 40px; }

/* ── 自动补全 ── */
.autocomplete-dropdown {
  position: fixed; z-index: 9999; width: 360px; max-height: 240px; overflow-y: auto;
  background: var(--ops-surface); border: 1px solid var(--ops-border); border-radius: 8px;
  box-shadow: 0 6px 20px rgba(0,0,0,0.12);
}
.suggest-loading, .suggest-empty { padding: 12px 16px; font-size: 13px; color: var(--ops-muted); }
.suggest-item { display: flex; align-items: center; gap: 8px; padding: 7px 14px; font-size: 13px; font-family: 'SF Mono', monospace; cursor: pointer; transition: background 0.1s; }
.suggest-item:hover, .suggest-item.active { background: var(--ops-primary-soft); }
.suggest-type-icon { width: 18px; height: 18px; border-radius: 4px; display: flex; align-items: center; justify-content: center; font-size: 10px; font-weight: 700; flex-shrink: 0; color: #fff; background: var(--ops-primary); }
.suggest-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.suggest-text .matched {
  color: var(--ops-primary);
  font-weight: 700;
}

/* ── 结果 ── */
.result-panel { background: var(--ops-surface); }
.result-tabs { display: flex; align-items: flex-end; border-bottom: 1px solid var(--ops-border); }
.tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 8px 16px;
  margin-bottom: -1px;
  color: var(--ops-primary);
  cursor: pointer;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 4px 4px 0 0;
  font-size: 16px;
  line-height: 24px;
}
.tab:hover { border-color: var(--ops-border-soft) var(--ops-border-soft) var(--ops-border); }
.tab.active { color: var(--ops-text); background: var(--ops-surface); border-color: var(--ops-border) var(--ops-border) var(--ops-surface); font-weight: 400; }
.tab:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
.tab-meta { margin-left: auto; padding: 0 16px 10px; font-size: 12px; color: var(--ops-muted); display: flex; gap: 16px; }
.result-content { padding-top: 16px; }
.table-content { padding: 16px; }
.panel-toolbar { display: flex; align-items: center; margin-bottom: 16px; padding: 0; }
.eval-control {
  --evaluation-height: 32px;
  display: inline-flex;
  align-items: stretch;
  gap: 0;
  height: var(--evaluation-height);
  max-width: 100%;
  box-sizing: border-box;
  border: 1px solid #6c757d;
  border-radius: 4px;
  background: var(--ops-surface);
}
.eval-control:focus-within { box-shadow: 0 0 0 3px var(--ops-primary-soft); }
.eval-arrow {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 32px;
  width: 32px;
  height: 100%;
  box-sizing: border-box;
  margin: 0;
  padding: 0 8px;
  border: 0;
  background: #6c757d;
  color: #fff;
  cursor: pointer;
}
.eval-arrow:first-child { border-radius: 3px 0 0 3px; }
.eval-arrow:last-child { border-radius: 0 3px 3px 0; }
.eval-arrow:hover { background: #5a6268; }
.eval-arrow:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -2px; }
.eval-arrow svg { width: 10px; height: 16px; }
.eval-control :deep(.eval-picker.el-input) {
  --el-input-height: calc(var(--evaluation-height) - 2px);
  --el-component-size-small: calc(var(--evaluation-height) - 2px);
  display: inline-flex;
  width: 220px;
  min-width: 0;
  height: 100%;
  margin: 0;
  vertical-align: top;
}
.eval-control :deep(.eval-picker .el-input__wrapper),
.eval-control :deep(.eval-picker .el-input__wrapper:hover),
.eval-control :deep(.eval-picker .el-input__wrapper.is-focus) {
  height: 100%;
  min-height: 0;
  box-sizing: border-box;
  border: 0;
  border-radius: 0;
  box-shadow: none;
  padding: 0 8px;
  background: var(--ops-surface);
}
.eval-control :deep(.eval-picker .el-input__prefix) { display: none; }
.eval-control :deep(.eval-picker .el-input__inner) {
  text-align: center;
  height: 100%;
  min-height: 0;
  padding: 0;
  font-size: 14px;
  line-height: calc(var(--evaluation-height) - 2px);
  color: var(--ops-text);
}
.eval-control :deep(.eval-picker .el-input__suffix-inner) { align-items: center; }
.eval-control :deep(.eval-picker .el-input__inner::placeholder) { color: var(--ops-muted); opacity: 1; }


.error-box { display: flex; align-items: center; gap: 8px; padding: 12px 16px; border-radius: 6px; }
.error-box { color: var(--ops-danger); background: var(--el-color-danger-light-9); }
.query-alert {
  margin: 0 0 16px;
  padding: 12px 20px;
  color: var(--ops-muted);
  font-size: 16px;
  line-height: 24px;
  background: var(--ops-surface-soft);
  border: 1px solid var(--ops-border-soft);
  border-radius: 4px;
}

.query-result-table :deep(.el-table__cell .cell) { white-space: nowrap; }
.metric-cell, .value-cell, .result-source { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.metric-cell { font-family: 'SF Mono', monospace; font-size: 13px; color: var(--ops-text); }
.result-source { max-width: 100%; color: var(--ops-muted); font-size: 13px; line-height: 20px; }
.value-cell { font-family: 'SF Mono', monospace; font-size: 13px; font-weight: 500; color: var(--ops-text); text-align: left; }

.empty-chart { display: flex; align-items: center; justify-content: center; height: 300px; color: var(--ops-muted); font-size: 13px; }

/* ── Graph params ── */
.graph-controls { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; margin: 0 0 16px; padding: 0 16px; }
.graph-input-group { display: inline-flex; align-items: stretch; }
.graph-controls input { box-sizing: border-box; height: 31px; min-width: 0; padding: 4px 8px; border: 1px solid var(--ops-border, #ced4da); border-radius: 3px; background: var(--ops-surface); color: var(--ops-text); font: 14px/1.5 Arial, sans-serif; }
.graph-controls input { text-align: center; }
.graph-controls input::placeholder { color: #6c757d; opacity: 1; }
.graph-controls input:focus { position: relative; z-index: 1; outline: 0; border-color: #80bdff; box-shadow: 0 0 0 3px rgb(0 123 255 / 25%); }
.graph-controls button { display: inline-flex; align-items: center; justify-content: center; box-sizing: border-box; height: 31px; padding: 4px 8px; border: 1px solid #6c757d; border-radius: 3px; background: #6c757d; color: #fff; font: 14px/1.5 Arial, sans-serif; white-space: nowrap; cursor: pointer; }
.graph-controls button:hover { background: #5a6268; border-color: #545b62; }
.graph-controls button.active { background: #545b62; border-color: #4e555b; box-shadow: inset 0 3px 5px rgb(0 0 0 / 13%); }
.graph-controls button:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 2px; }
.graph-controls button:disabled { opacity: .65; cursor: not-allowed; }
.graph-controls svg { width: 17.5px; height: 14px; fill: currentColor; }
.graph-input-group > :not(:first-child) { margin-left: -1px; border-top-left-radius: 0; border-bottom-left-radius: 0; }
.graph-input-group > :not(:last-child) { border-top-right-radius: 0; border-bottom-right-radius: 0; }
.range-input input { width: 70px; }
.time-input :deep(.graph-end-picker.el-input) { width: 230px; height: 31px; --el-input-height: 31px; }
.time-input :deep(.el-input__wrapper) { border-radius: 0; padding: 0 8px; background: var(--ops-surface); box-shadow: 0 0 0 1px var(--ops-border, #ced4da) inset; }
.time-input :deep(.el-input__wrapper.is-focus) { position: relative; z-index: 1; box-shadow: 0 0 0 1px #80bdff inset, 0 0 0 3px rgb(0 123 255 / 25%); }
.time-input :deep(.el-input__inner) { height: 29px; text-align: center; font: 14px/29px Arial, sans-serif; color: var(--ops-text); }
.time-input :deep(.el-input__inner::placeholder) { color: #6c757d; opacity: 1; }
.resolution-input { width: 80px; }

/* ── Metric legend ── */
.metric-legend {
  padding: 4px 16px 0;
  border-top: 1px solid var(--ops-border-soft);
}
.metric-legend-header {
  display: grid;
  grid-template-columns: 22px 180px minmax(0, 1fr);
  gap: 10px;
  min-height: 32px;
  align-items: center;
  color: var(--ops-muted);
  border-bottom: 1px solid var(--ops-border-soft);
  font-size: 12px;
  font-weight: 500;
}
.metric-legend-header::before { content: ''; }
.metric-legend-item {
  display: flex;
  width: 100%;
  min-height: 30px;
  align-items: center;
  gap: 10px;
  padding: 0;
  color: inherit;
  text-align: left;
  cursor: pointer;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--ops-border-soft);
  transition: background-color .16s ease, opacity .16s ease;
}
.metric-legend-item:hover { background: var(--ops-primary-soft); }
.metric-legend-item.active { background: var(--ops-primary-soft); }
.metric-legend-item.muted { opacity: .38; }
.metric-legend-item:focus-visible {
  position: relative;
  z-index: 1;
  outline: 2px solid var(--ops-primary);
  outline-offset: -2px;
}
.metric-legend-item:last-child { border-bottom: 0; }
.metric-legend-line {
  position: relative;
  width: 22px;
  height: 2px;
  flex: 0 0 22px;
  background: var(--series-color);
}
.metric-legend-line::after {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--series-color);
  content: '';
  transform: translate(-50%, -50%);
}
.metric-legend-source {
  width: 180px;
  flex: 0 0 180px;
  overflow: hidden;
  color: var(--ops-muted);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.metric-legend-name {
  flex: 1;
  min-width: 0;
  padding: 6px 0;
  color: var(--ops-muted);
  font-family: 'SF Mono', monospace;
  font-size: 12px;
  line-height: 18px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 720px) {
  .query-run-row {
    align-items: stretch;
    flex-direction: column;
  }
  .datasource-picker {
    min-width: 0;
    flex: 1;
  }
  .execute-button {
    width: 100%;
  }
  .eval-picker {
    width: 100%;
  }
}
</style>

<template>
  <div class="log-explorer">
    <header class="log-header">
      <div class="service-title"><span class="service-icon"><img :src="sourceLogo" :alt="isElasticsearch ? 'Elasticsearch' : 'Loki'" /></span><strong :title="source.name">{{ source.name }}</strong></div>
      <div class="time-controls">
        <el-date-picker v-model="range" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" :shortcuts="shortcuts" :disabled="loading" />
        <el-select v-model="limit" class="limit-select" aria-label="返回条数" :disabled="loading"><el-option v-for="count in [100, 500, 1000, 5000]" :key="count" :value="count" :label="`最多 ${count} 条`" /></el-select>
      </div>
    </header>
    <div class="search-row">
      <div class="query-editor"><span class="query-language">{{ isElasticsearch ? 'Lucene' : 'LogQL' }}</span><span class="line-number">1</span><el-input v-model="query" type="textarea" :autosize="{ minRows: 1, maxRows: 6 }" :maxlength="65536" :placeholder="queryHelp.placeholder" :aria-label="queryHelp.label" :disabled="loading" /></div>
      <el-button class="search-button" type="primary" :icon="Search" :loading="loading" :disabled="!sourceId || !query.trim()" @click="execute">查询 / 分析</el-button>
      <el-tooltip content="重新查询当前时间范围" placement="top"><el-button class="refresh-button" :icon="Refresh" aria-label="重新查询" :disabled="loading || !sourceId || !query.trim()" @click="execute" /></el-tooltip>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <el-alert v-if="limitReached" title="已达到返回条数上限，当前日志和分布仅包含返回结果。可缩小时间范围或增加筛选条件。" type="warning" :closable="false" show-icon />
    <section class="distribution">
      <div class="distribution-header">
        <button class="plain-button histogram-toggle" :aria-expanded="histogramVisible" @click="histogramVisible = !histogramVisible">日志条数：<strong>{{ entries.length.toLocaleString() }}</strong><el-icon><ArrowUp v-if="histogramVisible" /><ArrowDown v-else /></el-icon></button>
        <span class="result-summary"><template v-if="executed">查询结果 <span :class="limitReached ? 'limited' : 'success'">{{ limitReached ? '达到上限' : '查询完成' }}</span><span>耗时 {{ elapsed }} ms</span></template><template v-else>{{ loading ? '正在查询…' : '等待查询' }}</template></span>
      </div>
      <div v-if="histogramVisible" class="histogram" aria-label="本次返回日志的时间分布">
        <span v-if="executed" class="chart-caption">拖选时间范围以放大查询</span><span class="axis-max">{{ histogramMax }}</span><span class="axis-zero">0</span>
        <div class="plot" :class="{ 'is-selectable': executed && !loading }" @pointerdown="startSelection" @pointermove="moveSelection" @pointerup="finishSelection" @pointercancel="cancelSelection" @lostpointercapture="cancelSelection">
          <div class="plot-grid"></div>
          <div v-if="histogram.length" class="bars"><div v-for="(bin, index) in histogram" :key="index" class="bar-slot" :title="selection ? undefined : `${shortTime(bin.start)} – ${shortTime(bin.end)}：${bin.count} 条`"><div class="bar" :style="{ height: `${bin.count / histogramMax * 100}%` }"></div></div></div>
          <div v-if="selection" class="time-selection" :style="selectionStyle"><span class="selection-label">{{ shortTime(selectionRange[0]) }} – {{ shortTime(selectionRange[1]) }}</span></div>
        </div>
        <div class="time-axis"><span v-for="time in axisTimes" :key="time">{{ shortTime(time) }}</span></div>
      </div>
    </section>
    <div class="result-tabs"><span class="active-tab">原始日志</span></div>
    <section class="results-layout" :class="{ 'sidebar-hidden': !sidebarVisible }">
      <aside v-if="sidebarVisible" class="field-panel">
        <el-input v-model="fieldSearch" class="field-search" :prefix-icon="Search" placeholder="搜索字段名称" aria-label="搜索字段名称" clearable />
        <div class="field-heading">显示字段 <span>{{ displayedFields.length }}</span></div>
        <div class="field-list"><button v-for="key in displayedFields" :key="key" class="field-item plain-button active" :title="key" :aria-label="`移除显示字段 ${key}`" @click="toggleField(key)"><span class="field-type">T</span><span class="field-name">{{ key }}</span><span aria-hidden="true">−</span></button></div>
        <div class="field-heading">{{ isElasticsearch ? '日志字段' : '标签字段' }} <span>{{ availableFields.length }}</span></div>
        <div class="field-list"><button v-for="key in filteredFields" :key="key" class="field-item plain-button" :title="key" :aria-label="`添加显示字段 ${key}`" @click="toggleField(key)"><span class="field-type">T</span><span class="field-name">{{ key }}</span><span aria-hidden="true">+</span></button></div>
        <p v-if="!filteredFields.length" class="field-empty">{{ !labelFields.length ? (isElasticsearch ? '查询后显示日志字段' : '查询后显示日志标签') : !availableFields.length ? '全部字段已添加' : '无匹配字段' }}</p>
      </aside>
      <div class="log-results" v-loading="loading">
        <div class="result-toolbar">
          <el-button text :icon="sidebarVisible ? Fold : Expand" :aria-label="sidebarVisible ? '收起字段栏' : '展开字段栏'" @click="sidebarVisible = !sidebarVisible" />
          <el-radio-group v-model="viewMode" size="small" aria-label="日志显示模式"><el-radio-button value="table" label="table">表格</el-radio-button><el-radio-button value="raw" label="raw">原始</el-radio-button></el-radio-group>
          <label class="wrap-control">换行 <el-switch v-model="wrapLines" size="small" aria-label="日志换行" /></label>
          <button class="plain-button sort-button" @click="ascending = !ascending">时间<el-icon><ArrowUp v-if="ascending" /><ArrowDown v-else /></el-icon></button>
          <el-tooltip content="下载本次返回日志（NDJSON）"><el-button text :icon="Download" aria-label="下载日志" :disabled="!entries.length" @click="exportLogs" /></el-tooltip>
          <span class="result-count">{{ entries.length.toLocaleString() }} 条<span v-if="entries.length"> · {{ ascending ? '最早' : '最新' }}优先</span></span>
        </div>
        <template v-if="entries.length">
          <el-table v-if="viewMode === 'table'" :data="sortedEntries" class="log-table" :class="{ 'no-wrap': !wrapLines }" max-height="560" @row-click="openDetails">
            <el-table-column label="时间" width="210"><template #default="{ row }"><span class="timestamp">{{ formatTime(row.timestamp) }}</span></template></el-table-column>
            <template v-if="displayedFields.length"><el-table-column v-for="key in displayedFields" :key="key" :label="key" min-width="160"><template #default="{ row }"><span class="label-summary">{{ displayField(row, key) }}</span></template></el-table-column></template>
            <el-table-column label="日志内容" min-width="350"><template #default="{ row }"><pre class="log-line">{{ row.line }}</pre></template></el-table-column>
          </el-table>
          <div v-else class="raw-results" :class="{ 'no-wrap': !wrapLines }"><button v-for="(entry, index) in sortedEntries" :key="index" class="raw-row plain-button" @click="openDetails(entry)"><time>{{ formatTime(entry.timestamp) }}</time><pre class="log-line">{{ entry.line }}</pre></button></div>
        </template>
        <div v-else class="empty-state"><el-icon class="empty-icon"><Document /></el-icon><h3>{{ error ? '日志查询失败' : executed ? '当前查询条件下暂无日志' : '开始查询您的日志' }}</h3><p>{{ error ? '请检查错误提示，调整查询条件后重试。' : executed ? '请尝试扩大时间范围，或调整查询条件。' : '选择时间范围，输入查询条件，查看日志。' }}</p><code>{{ queryHelp.placeholder }}</code><span>支持点击日志行查看完整内容与元数据</span></div>
      </div>
    </section>
    <QueryHistoryDrawer ref="historyDrawer" @select="restoreQuery" />
    <el-drawer v-model="detailsVisible" title="日志详情" size="min(720px, 95vw)" style="border-radius: 0">
      <template v-if="selected"><p>{{ formatTime(selected.timestamp) }}</p><p class="hint">纳秒时间戳：{{ selected.timestamp }}</p><h4>标签</h4><pre class="details">{{ JSON.stringify(selected.labels, null, 2) }}</pre><template v-if="selected.metadata"><h4>元数据</h4><pre class="details">{{ JSON.stringify(selected.metadata, null, 2) }}</pre></template><template v-if="selected.fields"><h4>日志字段</h4><pre class="details">{{ JSON.stringify(selected.fields, null, 2) }}</pre></template><h4>日志内容</h4><pre class="details">{{ prettyLine(selected.line) }}</pre></template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Search, Refresh, ArrowDown, ArrowUp, Fold, Expand, Document, Download } from '@element-plus/icons-vue'
import request from '../../api/request'
import QueryHistoryDrawer from './QueryHistoryDrawer.vue'
import { flattenLogFields, formatLogField } from './log-fields.mjs'
import lokiLogo from '../../assets/loki-logo.svg'
import elasticsearchLogo from '../../assets/elasticsearch-logo.svg'
const historyDrawer = ref(null)
function openQueryHistory() { return historyDrawer.value?.openQueryHistory() }
function restoreQuery(item) { query.value = item.query }
defineExpose({ openQueryHistory })
const props = defineProps({ source: { type: Object, required: true } })
const isElasticsearch = computed(() => props.source.type === 'elasticsearch')
const sourceLogo = computed(() => isElasticsearch.value ? elasticsearchLogo : lokiLogo)
const sources = ref([]), sourceId = ref(null), initializing = ref(true), loading = ref(false)
const query = ref(''), limit = ref(500), range = ref([new Date(Date.now() - 3600000), new Date()])
const entries = ref([]), executed = ref(false), error = ref(''), limitReached = ref(false), elapsed = ref(0)
const selected = ref(null), detailsVisible = ref(false)
const viewMode = ref('table'), wrapLines = ref(true), ascending = ref(false)
const sidebarVisible = ref(true), histogramVisible = ref(true), fieldSearch = ref(''), displayedFields = ref([])
const resultRange = ref(null)
const selection = ref(null)
const selectionStyle = computed(() => selection.value ? {
  left: `${Math.min(selection.value.start, selection.value.end) * 100}%`,
  width: `${Math.abs(selection.value.end - selection.value.start) * 100}%`,
} : {})
const selectionRange = computed(() => {
  if (!selection.value) return []
  const { start, end, times } = selection.value
  return [Math.min(start, end), Math.max(start, end)].map(position => Math.round(times[0] + position * (times[1] - times[0])))
})
function selectionPosition(event, bounds) {
  return Math.max(0, Math.min(1, (event.clientX - bounds.left) / bounds.width))
}
function startSelection(event) {
  if (event.button !== 0 || !event.isPrimary || loading.value || !resultRange.value || selection.value) return
  const bounds = event.currentTarget.getBoundingClientRect()
  if (!bounds.width) return
  const position = selectionPosition(event, bounds)
  event.currentTarget.setPointerCapture(event.pointerId)
  selection.value = { pointerId: event.pointerId, start: position, end: position, bounds, times: [...resultRange.value] }
  event.preventDefault()
}
function moveSelection(event) {
  if (selection.value?.pointerId !== event.pointerId) return
  selection.value.end = selectionPosition(event, selection.value.bounds)
}
function cancelSelection() { selection.value = null }
function finishSelection(event) {
  if (selection.value?.pointerId !== event.pointerId) return
  moveSelection(event)
  const { start, end, bounds } = selection.value
  const [from, to] = selectionRange.value
  cancelSelection()
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
  // Ignore clicks and tiny accidental drags; preserve sub-second query ranges.
  if (loading.value || Math.abs(end - start) * bounds.width < 5 || to <= from) return
  range.value = [new Date(from), new Date(to)]
  execute()
}
const sortedEntries = computed(() => [...entries.value].sort((a, b) => {
  const left = BigInt(a.timestamp), right = BigInt(b.timestamp)
  return (left < right ? -1 : left > right ? 1 : 0) * (ascending.value ? 1 : -1)
}))
const entryFields = computed(() => new Map(entries.value.map(entry => [entry, isElasticsearch.value ? flattenLogFields(entry.fields || {}) : entry.labels || {}])))
const labelFields = computed(() => [...new Set([...entryFields.value.values()].flatMap(fields => Object.keys(fields)))].sort())
function displayField(entry, key) { return formatLogField(entryFields.value.get(entry)?.[key]) }
const availableFields = computed(() => labelFields.value.filter(key => !displayedFields.value.includes(key)))
const filteredFields = computed(() => availableFields.value.filter(key => key.toLowerCase().includes(fieldSearch.value.toLowerCase())))
const histogram = computed(() => {
  const times = resultRange.value
  if (!times) return []
  const [start, end] = times
  const width = (end - start) / 40
  const bins = Array.from({ length: 40 }, (_, i) => ({ start: start + i * width, end: start + (i + 1) * width, count: 0 }))
  for (const entry of entries.value) {
    const time = Number(BigInt(entry.timestamp) / 1000000n)
    if (time >= start && time <= end) bins[Math.min(39, Math.floor((time - start) / width))].count++
  }
  return bins
})
const histogramMax = computed(() => Math.max(1, ...histogram.value.map(bin => bin.count)))
const axisTimes = computed(() => resultRange.value ? Array.from({ length: 5 }, (_, i) => resultRange.value[0] + (resultRange.value[1] - resultRange.value[0]) * i / 4) : [])
const shortTime = time => new Date(time).toLocaleTimeString('zh-CN', { hour12: false })
function toggleField(key) { displayedFields.value = displayedFields.value.includes(key) ? displayedFields.value.filter(item => item !== key) : [...displayedFields.value, key] }
function exportLogs() {
  const blob = new Blob([sortedEntries.value.map(entry => JSON.stringify(entry)).join('\n')], { type: 'application/x-ndjson;charset=utf-8' })
  const url = URL.createObjectURL(blob), link = document.createElement('a')
  link.href = url; link.download = `${props.source.type}-logs.ndjson`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}

const selectedSource = computed(() => sources.value.find(source => source.id === sourceId.value))
const queryHelp = computed(() => isElasticsearch.value ? { label: 'Elasticsearch Lucene 查询', placeholder: 'level:ERROR AND service:nginx' } : { label: 'LogQL 日志查询', placeholder: '{job="nginx"}' })

const queriesBySource = new Map()
let previousSourceId
function changeSource() {
  if (previousSourceId) queriesBySource.set(previousSourceId, query.value)
  query.value = queriesBySource.get(sourceId.value) ?? (selectedSource.value?.type === 'loki' ? '' : '*')
  previousSourceId = sourceId.value
  resetResults()
}
let controller
const shortcuts = [15, 60, 360, 1440].map(minutes => ({ text: minutes < 60 ? `最近 ${minutes} 分钟` : `最近 ${minutes / 60} 小时`, value: () => [new Date(Date.now() - minutes * 60000), new Date()] }))
function resetResults() { cancelSelection(); resultRange.value = null; entries.value = []; executed.value = false; error.value = ''; limitReached.value = false }
function formatTime(ns) {
  const date = new Date(Number(BigInt(ns) / 1000000n))
  return date.toLocaleString('zh-CN', { hour12: false }) + '.' + String(date.getMilliseconds()).padStart(3, '0')
}
function prettyLine(line) { try { return JSON.stringify(JSON.parse(line), null, 2) } catch { return line } }
function openDetails(row) { selected.value = row; detailsVisible.value = true }
async function execute() {
  if (loading.value || !sourceId.value || !query.value.trim()) return
  const [start, end] = range.value || []
  if (!start || !end || end <= start || end - start > 31 * 86400000) { ElMessage.warning('请选择有效时间范围，最长 31 天'); return }
  resetResults(); loading.value = true
  controller = new AbortController()
  const started = performance.now()
  try {
    const response = await request({ url: '/query/logs', method: 'post', timeout: 35000, signal: controller.signal, data: { datasource_id: sourceId.value, query: query.value, start: start.toISOString(), end: end.toISOString(), limit: limit.value } })
    resultRange.value = [start.getTime(), end.getTime()]
    entries.value = response.data.entries;
    displayedFields.value = displayedFields.value.filter(key => labelFields.value.includes(key)); limitReached.value = response.data.limit_reached; executed.value = true
    elapsed.value = Math.round(performance.now() - started)
  } catch (e) { if (e.code !== 'ERR_CANCELED') error.value = e.response?.data?.message || e.message || '日志查询失败' }
  finally { loading.value = false }
}
onMounted(async () => {
  try { const response = { data: [props.source] }; sources.value = response.data; sourceId.value = sources.value[0]?.id || null; changeSource() }
  catch (e) { error.value = e.message || '读取日志数据源失败' }
  finally { initializing.value = false }
})
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.log-explorer { width: 100%; max-width: 100%; box-sizing: border-box; color: var(--ops-text); background: var(--ops-surface); min-width: 0; padding: 0 16px 16px; font-size: 13px; }
.log-header, .time-controls, .search-row, .distribution-header, .result-toolbar { display: flex; align-items: center; gap: 12px; }
.log-header { justify-content: space-between; flex-wrap: wrap; padding: 14px 0; }
.service-title { display: flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.service-icon { display: grid; place-items: center; flex: 0 0 28px; height: 28px; background: var(--ops-surface-soft); border: 1px solid var(--ops-border-soft); border-radius: 4px; color: var(--ops-primary); font-size: 16px; }
.service-icon img { display: block; width: 16px; height: 19px; object-fit: contain; }
.service-title strong { min-width: 0; max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 600; }
.time-controls { flex-wrap: wrap; justify-content: flex-end; min-width: 0; max-width: 100%; }
.time-controls :deep(.el-date-editor) { width: 360px; min-width: 0; max-width: 100%; flex: 0 1 360px; box-sizing: border-box; }
.time-controls :deep(.el-range-input) { min-width: 0; }
.limit-select { width: 140px; max-width: 100%; flex: 0 1 140px; min-width: 0; }
.search-row { align-items: stretch; gap: 8px; margin-bottom: 16px; }
.query-editor { display: flex; align-items: flex-start; flex: 1; min-width: 0; border: 1px solid var(--ops-border); border-radius: 3px; overflow: hidden; }
.query-editor:focus-within { border-color: var(--ops-primary); }
.query-language { color: var(--ops-primary); background: var(--ops-surface-soft); border-right: 1px solid var(--ops-border); padding: 10px 12px; align-self: stretch; font-size: 12px; }
.line-number { padding: 10px 8px 0 12px; color: var(--ops-subtle); font: 12px/20px monospace; }
.query-editor :deep(.el-textarea__inner) { box-shadow: none; border: 0; resize: none; padding: 10px 8px; font: 13px/20px Consolas, monospace; background: transparent; }
.search-button { height: auto; min-height: 40px; padding: 0 22px; border-radius: 3px; }
.refresh-button { margin: 0; height: auto; min-height: 40px; width: 40px; border-radius: 3px; }
.log-explorer > .el-alert { margin-bottom: 12px; }
.distribution-header { justify-content: space-between; min-height: 32px; }
.plain-button { border: 0; background: transparent; color: inherit; font: inherit; cursor: pointer; }
.plain-button:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -2px; }
.histogram-toggle { display: flex; align-items: center; gap: 8px; padding: 0; }
.result-summary { display: flex; gap: 16px; color: var(--ops-muted); font-size: 12px; }
.success { color: var(--ops-success); }.limited { color: var(--ops-warning); }
.histogram { position: relative; height: 128px; padding: 26px 0 24px 36px; box-sizing: border-box; }
.chart-caption { position: absolute; top: 4px; left: 36px; color: var(--ops-subtle); font-size: 11px; }
.axis-max, .axis-zero { position: absolute; left: 0; font-size: 11px; color: var(--ops-subtle); }.axis-max { top: 24px; }.axis-zero { bottom: 21px; }
.plot { position: relative; height: 100%; border-bottom: 1px solid var(--ops-border); }
.plot.is-selectable { cursor: crosshair; touch-action: none; user-select: none; }
.time-selection { position: absolute; top: 0; bottom: 0; pointer-events: none; background: color-mix(in srgb, var(--ops-primary) 20%, transparent); border-inline: 1px solid var(--ops-primary); box-sizing: border-box; }
.selection-label { position: absolute; top: 2px; left: 50%; transform: translateX(-50%); white-space: nowrap; padding: 2px 5px; background: var(--ops-surface); color: var(--ops-text); border: 1px solid var(--ops-border); border-radius: 3px; font-size: 11px; }
.plot-grid { position: absolute; inset: 0; border-top: 1px dashed var(--ops-border-soft); background: repeating-linear-gradient(to top, transparent 0, transparent 25px, var(--ops-border-soft) 26px); }
.bars { display: flex; height: 100%; gap: 3px; position: relative; }.bar-slot { flex: 1; display: flex; align-items: flex-end; min-width: 0; }.bar { width: 100%; background: var(--ops-primary); opacity: .75; }.bar-slot:hover .bar { opacity: 1; }
.time-axis { display: flex; justify-content: space-between; padding-top: 6px; color: var(--ops-muted); font-size: 11px; }
.result-tabs { display: flex; align-items: center; gap: 24px; border-bottom: 1px solid var(--ops-border); margin-top: 10px; height: 43px; }.active-tab { color: var(--ops-primary); font-weight: 600; height: 100%; display: flex; align-items: center; border-bottom: 2px solid var(--ops-primary); }
.results-layout { display: grid; grid-template-columns: 220px minmax(0, 1fr); min-height: 420px; }.results-layout.sidebar-hidden { grid-template-columns: minmax(0, 1fr); }
.field-panel { padding: 14px 12px 14px 0; border-right: 1px solid var(--ops-border); }.field-heading { display: flex; justify-content: space-between; margin: 20px 0 10px; font-weight: 600; }.field-heading span { font-weight: 400; color: var(--ops-subtle); }
.field-search :deep(.el-input__wrapper) { min-height: 40px; padding: 0 12px; border-radius: 6px; background: var(--ops-surface-soft); box-shadow: 0 0 0 1px var(--ops-border) inset; transition: box-shadow .15s, background-color .15s; }
.field-search :deep(.el-input__wrapper:hover) { box-shadow: 0 0 0 1px var(--ops-muted) inset; }
.field-search :deep(.el-input__wrapper.is-focus) { background: var(--ops-surface); box-shadow: 0 0 0 2px var(--ops-primary) inset; }
.field-search :deep(.el-input__inner) { font-size: 13px; }
.field-search :deep(.el-input__prefix) { color: var(--ops-muted); font-size: 16px; }
.active { color: var(--ops-primary); }
.field-list { max-height: 400px; overflow: auto; }.field-item { display: flex; align-items: center; gap: 8px; width: 100%; padding: 8px; text-align: left; }.field-item:hover, .raw-row:hover { background: var(--ops-surface-soft); }.field-type { color: var(--ops-subtle); font: 12px monospace; }.field-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.field-empty { font-size: 12px; color: var(--ops-subtle); padding: 4px 8px; }
.log-results { min-width: 0; }.result-toolbar { flex-wrap: wrap; min-height: 48px; padding: 6px 12px; gap: 14px; border-bottom: 1px solid var(--ops-border-soft); box-sizing: border-box; }.result-toolbar > .el-button { margin: 0; padding: 5px; }.wrap-control { display: flex; align-items: center; gap: 6px; color: var(--ops-muted); font-size: 12px; }.sort-button { display: flex; align-items: center; gap: 4px; font-size: 12px; }.result-count { margin-left: auto; color: var(--ops-subtle); font-size: 12px; }
.log-table { font-size: 12px; }.log-table :deep(.el-table__row) { cursor: pointer; }.log-table :deep(th.el-table__cell) { background: var(--ops-surface-soft); }.timestamp { color: var(--ops-muted); font-size: 12px; }.label-summary { overflow-wrap: anywhere; font-size: 12px; }.log-line { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font: 12px/1.8 Consolas, Monaco, monospace; max-height: 150px; overflow: auto; }.no-wrap .log-line { white-space: pre; overflow-wrap: normal; }
.raw-results { max-height: 560px; overflow: auto; }.raw-row { display: flex; width: 100%; gap: 20px; text-align: left; padding: 12px 16px; border-bottom: 1px solid var(--ops-border-soft); }.raw-row time { flex: 0 0 190px; color: var(--ops-muted); font-size: 12px; padding-top: 4px; }.raw-row .log-line { flex: 1; min-width: 0; }
.empty-state { min-height: 350px; display: flex; flex-direction: column; justify-content: center; align-items: center; text-align: center; padding: 24px; box-sizing: border-box; }.empty-icon { font-size: 48px; color: var(--ops-primary); opacity: .5; margin-bottom: 6px; }.empty-state h3 { font-size: 15px; font-weight: 500; margin: 12px 0 0; }.empty-state p { color: var(--ops-muted); font-size: 12px; line-height: 1.8; }.empty-state code { padding: 8px 18px; background: var(--ops-surface-soft); border: 1px solid var(--ops-border-soft); color: var(--ops-muted); }.empty-state > span { color: var(--ops-subtle); font-size: 12px; margin-top: 20px; }
.hint { color: var(--ops-muted); font-size: 13px; }.details { white-space: pre-wrap; overflow-wrap: anywhere; background: var(--ops-surface-soft); padding: 12px; border: 1px solid var(--ops-border); border-radius: 4px; }
@media (max-width: 900px) { .time-controls { width: 100%; }.time-controls :deep(.el-date-editor) { flex: 1 1 360px; }.results-layout { grid-template-columns: 180px minmax(0, 1fr); }}
@media (max-width: 600px) { .log-explorer { padding: 0 8px 8px; }.time-controls :deep(.el-date-editor) { width: 100%; min-width: 0; flex-basis: 100%; }.search-row { flex-wrap: wrap; }.query-editor { flex-basis: 100%; }.search-button { flex: 1; min-height: 36px; }.results-layout { grid-template-columns: minmax(0, 1fr); }.field-panel { border-right: 0; border-bottom: 1px solid var(--ops-border); padding-right: 0; }.field-list { max-height: 130px; }.result-summary { gap: 8px; }.raw-row { flex-direction: column; gap: 4px; }.raw-row time { flex-basis: auto; }.time-axis { font-size: 9px; } }
</style>

<template>
  <div class="page-container log-query">
    <div class="query-toolbar">
      <el-select v-if="!source" v-model="sourceId" filterable placeholder="选择日志数据源" aria-label="日志数据源" :disabled="loading" @change="changeSource">
        <el-option v-for="source in sources" :key="source.id" :value="source.id" :label="`${source.name} · ${sourceTypeLabel(source.type)}`" />
      </el-select>
      <el-date-picker v-model="range" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" :shortcuts="shortcuts" :disabled="loading" />
      <el-select v-model="limit" aria-label="返回条数" :disabled="loading">
        <el-option v-for="count in [100, 500, 1000, 5000]" :key="count" :value="count" :label="`最多 ${count} 条`" />
      </el-select>
    </div>
    <el-alert v-if="error && !sources.length" :title="error" type="error" :closable="false" show-icon />
    <el-empty v-else-if="!initializing && !sources.length" description="暂无启用的日志数据源，请在接入中心添加 Loki、Elasticsearch 或 ClickHouse" />
    <template v-else>
      <el-input v-model="query" type="textarea" :rows="3" :maxlength="65536" :placeholder="queryHelp.placeholder" :aria-label="queryHelp.label" :disabled="loading" @keydown.ctrl.enter.prevent="execute" @keydown.meta.enter.prevent="execute" />
      <p class="hint query-help">{{ queryHelp.description }}</p>
      <div class="query-actions">
        <span class="hint">{{ queryHelp.label }} · Ctrl / ⌘ + Enter 执行 · 最新日志优先</span>
        <el-button type="primary" :loading="loading" :disabled="!sourceId || !query.trim()" @click="execute">查询日志</el-button>
      </div>
      <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
      <el-alert v-if="limitReached" title="已达到返回条数上限，可缩小时间范围或增加筛选条件以查看其他日志。" type="warning" :closable="false" show-icon />
      <div v-if="executed" class="hint">返回 {{ entries.length }} 条日志 · 耗时 {{ elapsed }} ms</div>
      <el-table v-if="executed || loading" v-loading="loading" :data="entries" empty-text="当前条件下没有日志" max-height="650" @row-click="openDetails">
        <el-table-column label="时间" width="230"><template #default="{ row }">{{ formatTime(row.timestamp) }}</template></el-table-column>
        <el-table-column label="标签" width="220"><template #default="{ row }"><span class="label-summary">{{ labelSummary(row.labels) }}</span></template></el-table-column>
        <el-table-column label="日志内容" min-width="350"><template #default="{ row }"><pre class="log-line">{{ row.line }}</pre></template></el-table-column>
      </el-table>
      <el-empty v-else-if="!error" description="选择数据源与时间范围，输入查询条件开始查询" />
    </template>
    <el-drawer v-model="detailsVisible" title="日志详情" size="min(720px, 95vw)">
      <template v-if="selected">
        <p>{{ formatTime(selected.timestamp) }}</p>
        <p class="hint">纳秒时间戳：{{ selected.timestamp }}</p>
        <h4>标签</h4><pre class="details">{{ JSON.stringify(selected.labels, null, 2) }}</pre>
        <template v-if="selected.metadata"><h4>元数据</h4><pre class="details">{{ JSON.stringify(selected.metadata, null, 2) }}</pre></template>
        <template v-if="selected.fields"><h4>日志字段</h4><pre class="details">{{ JSON.stringify(selected.fields, null, 2) }}</pre></template>
        <h4>日志内容</h4><pre class="details">{{ prettyLine(selected.line) }}</pre>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { ElMessage } from 'element-plus'
import request from '../../api/request'
const props = defineProps({ source: { type: Object, default: null } })
const sources = ref([]), sourceId = ref(null), initializing = ref(true), loading = ref(false)
const query = ref(''), limit = ref(500), range = ref([new Date(Date.now() - 3600000), new Date()])
const entries = ref([]), executed = ref(false), error = ref(''), limitReached = ref(false), elapsed = ref(0)
const selected = ref(null), detailsVisible = ref(false)
const sourceTypeLabel = type => ({ loki: 'Loki', elasticsearch: 'Elasticsearch', clickhouse: 'ClickHouse' }[type] || type)
const selectedSource = computed(() => sources.value.find(source => source.id === sourceId.value))
const queryHelp = computed(() => ({
  loki: { label: 'LogQL 日志查询', placeholder: '{app="nginx"} |= "error"', description: '使用标签选择日志流，可通过 |= 筛选正文。' },
  elasticsearch: { label: 'Elasticsearch 搜索语法', placeholder: 'error AND service.name:api', description: '支持关键词、字段条件和 AND / OR，输入 * 查询所选时间内全部日志。' },
  clickhouse: { label: 'ClickHouse 关键词查询', placeholder: 'error', description: '按日志正文包含关键词匹配，不区分大小写；输入 * 查询全部日志。' },
}[selectedSource.value?.type] || { label: '日志查询', placeholder: '输入查询条件', description: '' }))
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
function resetResults() { entries.value = []; executed.value = false; error.value = ''; limitReached.value = false }
function formatTime(ns) {
  const date = new Date(Number(BigInt(ns) / 1000000n))
  return date.toLocaleString('zh-CN', { hour12: false }) + '.' + String(date.getMilliseconds()).padStart(3, '0')
}
function labelSummary(labels) { return Object.entries(labels || {}).map(([key, value]) => `${key}=${value}`).join(' · ') }
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
    entries.value = response.data.entries; limitReached.value = response.data.limit_reached; executed.value = true
    elapsed.value = Math.round(performance.now() - started)
  } catch (e) { if (e.code !== 'ERR_CANCELED') error.value = e.response?.data?.message || e.message || '日志查询失败' }
  finally { loading.value = false }
}
onMounted(async () => {
  try { const response = props.source ? { data: [props.source] } : await request({ url: '/query/logs/datasources' }); sources.value = response.data; sourceId.value = sources.value[0]?.id || null; changeSource() }
  catch (e) { error.value = e.message || '读取日志数据源失败' }
  finally { initializing.value = false }
})
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.log-query { display: flex; flex-direction: column; gap: 16px; }
.query-toolbar { display: flex; gap: 12px; flex-wrap: wrap; }
.query-toolbar > .el-select { width: 220px; }
.query-toolbar :deep(.el-date-editor) { flex: 1; min-width: 320px; }
.query-actions { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.query-help { margin: -6px 0 0; }
.hint { color: var(--ops-muted); font-size: 13px; }
.log-line { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 12px; max-height: 100px; overflow: hidden; cursor: pointer; }
.label-summary { font-size: 12px; overflow-wrap: anywhere; }
.details { white-space: pre-wrap; overflow-wrap: anywhere; background: var(--ops-surface); padding: 12px; border: 1px solid var(--ops-border); border-radius: 6px; }
@media (max-width: 600px) { .query-toolbar > .el-select { width: 100%; } .query-toolbar :deep(.el-date-editor) { min-width: 0; width: 100%; } }
</style>

<template>
  <div class="page-container explore">
    <div class="explore-toolbar">
      <el-select class="source-select" v-model="selectedId" filterable placeholder="选择数据源" aria-label="查询数据源" :loading="loading" @change="selectSource">
        <template #prefix>
          <img v-if="selected && sourceLogos[selected.type]" :src="sourceLogos[selected.type]" class="source-logo" :class="{ 'vm-logo': selected.type === 'victoriametrics' }" alt="" />
        </template>
          <el-option v-for="item in sources" :key="item.id" :label="item.name" :value="item.id">
            <span class="source-option"><img v-if="sourceLogos[item.type]" :src="sourceLogos[item.type]" class="source-logo" :class="{ 'vm-logo': item.type === 'victoriametrics' }" alt="" /><span>{{ item.name }}</span></span>
          </el-option>
      </el-select>
      <el-select class="mode-select" v-if="selected?.modes.length > 1" v-model="mode" aria-label="查询模式" @change="saveLocation">
        <el-option v-for="item in selected.modes" :key="item" :label="modeName(item)" :value="item" />
      </el-select>
      <div class="explore-actions">
        <el-tooltip v-if="mode === 'metrics' || (mode === 'logs' && ['loki', 'elasticsearch'].includes(selected?.type))" content="历史查询" placement="bottom">
          <el-button class="history-button" aria-label="历史查询" :disabled="!historyView" @click="historyView?.openQueryHistory()">
          <svg class="history-icon" viewBox="64 64 896 896" aria-hidden="true">
            <path d="M536.1 273H488c-4.4 0-8 3.6-8 8v275.3c0 2.6 1.2 5 3.3 6.5l165.3 120.7c3.6 2.6 8.6 1.9 11.2-1.7l28.6-39c2.7-3.7 1.9-8.7-1.7-11.2L544.1 528.5V281c0-4.4-3.6-8-8-8zm219.8 75.2l156.8 38.3c5 1.2 9.9-2.6 9.9-7.7l.8-161.5c0-6.7-7.7-10.5-12.9-6.3L752.9 334.1a8 8 0 003 14.1zm167.7 301.1l-56.7-19.5a8 8 0 00-10.1 4.8c-1.9 5.1-3.9 10.1-6 15.1-17.8 42.1-43.3 80-75.9 112.5a353 353 0 01-112.5 75.9 352.18 352.18 0 01-137.7 27.8c-47.8 0-94.1-9.3-137.7-27.8a353 353 0 01-112.5-75.9c-32.5-32.5-58-70.4-75.9-112.5A353.44 353.44 0 01171 512c0-47.8 9.3-94.2 27.8-137.8 17.8-42.1 43.3-80 75.9-112.5a353 353 0 01112.5-75.9C430.6 167.3 477 158 524.8 158s94.1 9.3 137.7 27.8A353 353 0 01775 261.7c10.2 10.3 19.8 21 28.6 32.3l59.8-46.8C784.7 146.6 662.2 81.9 524.6 82 285 82.1 92.6 276.7 95 516.4 97.4 751.9 288.9 942 524.8 942c185.5 0 343.5-117.6 403.7-282.3 1.5-4.2-.7-8.9-4.9-10.4z" />
          </svg>
          </el-button>
        </el-tooltip>
      </div>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" />
    <QueryDatasourceEmpty v-if="!loading && !sources.length && !error" />
    <template v-if="selected">
      <div v-if="mode === 'metrics'" class="metrics-view">
        <InstantQuery ref="metricsView" :key="`metrics-${selected.id}`" :source="selected" />
      </div>
      <div v-else-if="mode === 'logs' && selected.type === 'loki'" class="loki-view">
        <LokiQuery ref="lokiView" :key="`loki-${selected.id}`" :source="selected" />
      </div>
      <div v-else-if="mode === 'logs' && selected.type === 'elasticsearch'" class="elasticsearch-view">
        <ElasticsearchQuery ref="esView" :key="`es-${selected.id}`" :source="selected" />
      </div>
      <div v-else-if="selected.type === 'clickhouse'" class="clickhouse-view">
        <SQLQuery v-if="mode === 'sql'" :key="`sql-${selected.id}`" :source="selected" />
        <ClickHouseLogQuery v-else :key="`ch-logs-${selected.id}`" :source="selected" />
      </div>
      <div v-else-if="mode === 'traces'" class="trace-view">
        <TraceQuery :key="`traces-${selected.id}`" :source="selected" />
      </div>
    </template>
  </div>
</template>
<script setup>
import QueryDatasourceEmpty from '../../components/QueryDatasourceEmpty.vue'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import request from '../../api/request'
import InstantQuery from './InstantQuery.vue'
import LokiQuery from './LokiQuery.vue'
import ElasticsearchQuery from './ElasticsearchQuery.vue'
import ClickHouseLogQuery from './ClickHouseLogQuery.vue'
import SQLQuery from './SQLQuery.vue'
import TraceQuery from './TraceQuery.vue'
import prometheusLogo from '../../assets/prometheus-logo.7aa022e5.svg'
import victoriaMetricsLogo from '../../assets/victoriametrics-logo.svg'
import lokiLogo from '../../assets/loki-logo.svg'
import elasticsearchLogo from '../../assets/elasticsearch-logo.svg'
import clickhouseLogo from '../../assets/clickhouse-logo.svg'
import jaegerLogo from '../../assets/jaeger-logo.svg'
const sourceLogos = { prometheus: prometheusLogo, victoriametrics: victoriaMetricsLogo, loki: lokiLogo, elasticsearch: elasticsearchLogo, clickhouse: clickhouseLogo, jaeger: jaegerLogo }
const metricsView = ref(null), lokiView = ref(null), esView = ref(null)
const historyView = computed(() => mode.value === 'metrics' ? metricsView.value : selected.value?.type === 'elasticsearch' ? esView.value : lokiView.value)
const route = useRoute(), router = useRouter()
const sources = ref([]), selectedId = ref(null), mode = ref(''), error = ref(''), loading = ref(false)
const selected = computed(() => sources.value.find(item => item.id === selectedId.value))
const modeName = value => ({ metrics: '指标', logs: '日志', sql: 'SQL', traces: 'Trace' }[value] || value)

function saveLocation() { router.replace({ path: '/monitor/query', query: { datasource: String(selectedId.value), mode: mode.value } }) }
function selectSource() { mode.value = selected.value?.modes[0] || ''; saveLocation() }
function applyLocation() {
  const requested = Number(route.query.datasource)
  const chosen = sources.value.find(item => item.id === requested) || sources.value.find(item => item.modes.includes(route.query.kind)) || sources.value[0]
  selectedId.value = chosen?.id || null
  mode.value = chosen?.modes.includes(route.query.mode) ? route.query.mode : chosen?.modes.includes(route.query.kind) ? route.query.kind : chosen?.modes[0] || ''
}
watch(() => [route.query.datasource, route.query.mode, route.query.kind], applyLocation)
async function fetchSources() {
  loading.value = true; error.value = ''
  try {
    const response = await request({ url: '/query/datasources' }); sources.value = response.data
    applyLocation()
  } catch (e) { error.value = e.message || '读取数据源失败' }
  finally { loading.value = false }
}
onMounted(fetchSources)
</script>
<style scoped>
.explore { display: flex; flex-direction: column; gap: 14px; min-width: 0; }
.explore-toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 0 0 12px; }
.source-logo { width: 20px; height: 20px; object-fit: contain; flex-shrink: 0; }
.source-option { display: flex; align-items: center; gap: 9px; min-width: 0; height: 100%; }
.source-option > span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.vm-logo { filter: invert(1); }
:global(html.dark) .vm-logo { filter: none; }
.history-button { width: 32px; padding: 0; }
.history-icon { width: 17px; height: 17px; fill: currentColor; }
.source-select { width: 280px; max-width: 100%; }
.source-select :deep(.el-select__wrapper) { min-height: 42px; padding: 8px 12px; border-radius: 6px; background: var(--ops-surface-soft); box-shadow: 0 0 0 1px var(--ops-border) inset; transition: box-shadow .15s, background-color .15s; }
.source-select :deep(.el-select__wrapper:hover) { box-shadow: 0 0 0 1px var(--ops-muted) inset; }
.source-select :deep(.el-select__wrapper.is-focused) { background: var(--ops-surface); box-shadow: 0 0 0 2px var(--ops-primary) inset; }
.source-select :deep(.el-select__selected-item) { font-size: 14px; font-weight: 500; }
.mode-select { width: 120px; }
.explore-actions { display: flex; align-items: center; gap: 8px; margin-left: auto; flex-wrap: wrap; }
.explore-actions :deep(.el-button + .el-button) { margin-left: 0; }
@media(max-width:600px) {
  .source-select { flex: 1; min-width: 180px; }
  .explore-actions { width: 100%; margin-left: 0; }
}
</style>

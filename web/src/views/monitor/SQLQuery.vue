<template>
  <div class="sql-query">
    <header class="sql-header"><div class="service-title"><span class="service-icon"><img :src="clickhouseLogo" alt="ClickHouse" /></span><strong :title="source.name">{{ source.name }}</strong></div><span>SQL 查询</span></header>
    <section class="sql-editor-panel">
      <div class="editor-heading"><strong>SQL</strong><el-button text :disabled="loading" @click="query = timeExample">插入时间过滤示例</el-button></div>
      <CodeEditor v-model="query" language="sql" height="260px" aria-label="ClickHouse SQL 查询" />
    </section>
    <div class="toolbar">
      <el-date-picker v-model="range" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" :disabled="loading" />
      <el-select v-model="limit" class="row-limit" aria-label="返回行数" :disabled="loading"><el-option v-for="n in [100,500,1000,5000]" :key="n" :value="n" :label="`最多 ${n} 行`" /></el-select>
      <el-button type="primary" :loading="loading" :disabled="!source.id || !query.trim()" @click="execute">执行 SQL</el-button>
    </div>
    <p class="hint">时间范围仅作用于 SQL 中的 {from:Int64}、{to:Int64} 参数（Unix 秒）。</p>
    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <el-alert v-if="result?.limit_reached" title="已达到行数上限，可调整条件或增加返回行数。" type="warning" :closable="false" show-icon />
    <section v-loading="loading" class="sql-results">
      <div class="results-heading"><strong>查询结果</strong><span>{{ result?.rows.length ?? 0 }} 行<span v-if="result"> · {{ elapsed }} ms</span></span></div>
      <el-table v-if="result" :data="result.rows" max-height="650" empty-text="查询没有返回数据">
        <el-table-column v-for="(column, index) in result.columns" :key="index" :label="column.name" min-width="180"><template #default="{ row }"><span class="cell">{{ cell(row[column.name]) }}</span></template></el-table-column>
      </el-table>
      <el-empty v-else :description="error ? '查询失败，请调整 SQL 后重试' : '输入 SQL 并执行，查看查询结果'" :image-size="64" />
    </section>
  </div>
</template>
<script setup>
import { defineAsyncComponent, onBeforeUnmount, ref } from 'vue'
const CodeEditor = defineAsyncComponent(() => import('../../components/CodeEditor.vue'))
import request from '../../api/request'
import clickhouseLogo from '../../assets/clickhouse-logo.svg'
const props = defineProps({ source: { type: Object, required: true } })
const query = ref('SELECT 1 AS value'), range = ref([new Date(Date.now()-3600000),new Date()]), limit = ref(500), loading = ref(false), error = ref(''), result = ref(null), elapsed = ref(0)
const timeExample = 'SELECT *\nFROM your_table\nWHERE Timestamp >= toDateTime({from:Int64})\n  AND Timestamp < toDateTime({to:Int64})\nORDER BY Timestamp DESC'
const cell = value => value === null ? 'NULL' : typeof value === 'object' ? JSON.stringify(value) : String(value ?? '')
let controller
async function execute() {
  if (loading.value || !props.source.id) return
  error.value = ''; result.value = null
  const [start,end] = range.value || []
  if (!query.value.trim() || query.value.length > 65536 || !start || !end || end <= start) { error.value = '请输入有效 SQL（最多 64 KB）和时间范围'; return }
  controller = new AbortController(); loading.value = true; const now = performance.now()
  try { const response = await request({ url:'/query/sql',method:'post',timeout:35000,signal:controller.signal,data:{ datasource_id:props.source.id,query:query.value,start:start.toISOString(),end:end.toISOString(),limit:limit.value } }); result.value=response.data; elapsed.value=Math.round(performance.now()-now) }
  catch(e) { if(e.code!=='ERR_CANCELED') error.value=e.response?.data?.message || e.message }
  finally {loading.value=false}
}
onBeforeUnmount(()=>controller?.abort())
</script>
<style scoped>
.sql-query { display: flex; flex-direction: column; gap: 14px; width: 100%; min-width: 0; max-width: 100%; padding: 16px; box-sizing: border-box; background: var(--ops-surface); }
.sql-header, .editor-heading, .results-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-width: 0; }
.sql-header strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; }
.sql-header > span, .results-heading > span { color: var(--ops-muted); font-size: 12px; flex-shrink: 0; }
.sql-editor-panel, .sql-results { min-width: 0; border: 1px solid var(--ops-border); }
.editor-heading, .results-heading { min-height: 40px; padding: 0 12px; background: var(--ops-surface-soft); border-bottom: 1px solid var(--ops-border); font-size: 13px; }
.toolbar { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; min-width: 0; }
.toolbar :deep(.el-date-editor) { width: 360px; min-width: 0; max-width: 100%; flex: 0 1 360px; box-sizing: border-box; }
.toolbar :deep(.el-range-input) { min-width: 0; }
.row-limit { width: 160px; max-width: 100%; }
.toolbar > .el-button { margin-left: auto; }
.hint { color: var(--ops-muted); font-size: 12px; line-height: 1.6; margin: 0; }
.cell { white-space: pre-wrap; overflow-wrap: anywhere; font-family: Consolas, monospace; }
@media (max-width: 600px) { .sql-query { padding: 8px; }.toolbar :deep(.el-date-editor) { flex-basis: 100%; width: 100%; } }
.service-title { display: flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.service-icon { display: grid; place-items: center; flex: 0 0 28px; height: 28px; background: var(--ops-surface-soft); border: 1px solid var(--ops-border-soft); border-radius: 4px; }
.service-icon img { display: block; width: 18px; height: 20px; object-fit: contain; }
.service-title strong { min-width: 0; max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 600; }
</style>

<template>
  <div class="history-page" v-loading="loading">
    <div class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="拨测历史记录筛选">
        <el-input v-model="keyword" class="task-search" clearable :prefix-icon="Search" placeholder="搜索节点或结果" aria-label="搜索执行节点或结果内容" @input="handleKeywordInput" @keyup.enter="searchRecords" @clear="searchRecords" />
        <RecordTimeRange v-model="timeRange" @change="searchRecords" />
        <el-button :disabled="!hasFilters" @click="resetFilters">清空</el-button>
      </div>
      <div class="page-header-actions"><el-button :icon="Refresh" :loading="loading" @click="loadRecords">刷新</el-button></div>
    </div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <el-card shadow="never">
      <el-table :data="records" stripe>
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small">{{ row.status === 'success' ? '正常' : row.status === 'error' ? '错误' : '失败' }}</el-tag></template></el-table-column>
        <el-table-column prop="node_name" label="执行节点" min-width="140"><template #default="{ row }">{{ row.node_name || (row.node_type === 'direct' ? '本地节点' : '—') }}</template></el-table-column>
        <el-table-column v-if="item.type === 'http'" label="HTTP 状态码" width="120"><template #default="{ row }">{{ statusCode(row.result) }}</template></el-table-column>
        <el-table-column v-if="item.type === 'script'" label="退出码" width="90"><template #default="{ row }">{{ parseResult(row.result).exit_code ?? '—' }}</template></el-table-column>
        <el-table-column label="耗时" width="120"><template #default="{ row }">{{ row.duration_ms }} ms</template></el-table-column>
        <el-table-column label="结果详情" width="110"><template #default="{ row }"><el-button link type="primary" @click="openResult(row)">查看详情</el-button></template></el-table-column>
        <el-table-column label="执行时间" min-width="180"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
        <template #empty><el-empty description="暂无符合条件的拨测记录" :image-size="72" /></template>
      </el-table>
      <div v-if="total" class="history-pagination">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next" @current-change="loadRecords" @size-change="changePageSize" />
      </div>
    </el-card>
    <el-drawer v-model="resultVisible" title="拨测结果详情" size="min(720px, 100vw)" :style="{ borderRadius: '0' }" destroy-on-close>
      <template v-if="selectedRecord">
        <el-descriptions :column="1" border class="result-meta">
          <el-descriptions-item label="任务">{{ item.name }}</el-descriptions-item>
          <el-descriptions-item label="执行节点">{{ selectedRecord.node_name || '—' }}</el-descriptions-item>
          <el-descriptions-item label="执行时间">{{ formatTime(selectedRecord.created_at) }}</el-descriptions-item>
          <el-descriptions-item label="状态">{{ selectedRecord.status === 'success' ? '正常' : selectedRecord.status === 'error' ? '错误' : '失败' }}</el-descriptions-item>
          <el-descriptions-item label="耗时">{{ selectedRecord.duration_ms }} ms</el-descriptions-item>
        </el-descriptions>
        <CertificateExpiryResult v-if="item.type === 'certificate_expiry'" :result="selectedRecord.result" />
        <div class="result-code-heading"><span>结果内容</span><el-tooltip content="复制" placement="top"><el-button size="small" :icon="CopyDocument" aria-label="复制结果内容" @click="copyResult" /></el-tooltip></div>
        <CodeEditor :model-value="formatResult(selectedRecord.result)" :language="resultLanguage" read-only height="420px" aria-label="拨测结果内容（只读）" />
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { useRoute } from 'vue-router'
import { CopyDocument, Refresh, Search } from '@element-plus/icons-vue'
import { getInspectionRecords, getInspectionRuleGroup } from '../../api/inspection'
import { usePageBreadcrumb } from '../../composables/usePageBreadcrumb'
import { copyText } from '../../utils/clipboard'
import RecordTimeRange from '../../components/RecordTimeRange.vue'
import CertificateExpiryResult from '../../components/CertificateExpiryResult.vue'
const CodeEditor = defineAsyncComponent(() => import('../../components/CodeEditor.vue'))

const route = useRoute()
const item = ref({}), records = ref([]), total = ref(0)
const groupName = ref('')
usePageBreadcrumb(() => '历史记录', () => [
  { title: '拨测任务', path: '/inspection/items' },
  ...(item.value.group_id ? [{ title: groupName.value || '规则组', path: `/inspection/items/groups/${item.value.group_id}` }] : []),
])
watch(() => item.value.group_id, async (groupID, previous, onCleanup) => {
  let cancelled = false
  onCleanup(() => { cancelled = true })
  groupName.value = ''
  if (!groupID) return
  try {
    const { data } = await getInspectionRuleGroup(groupID)
    if (!cancelled) groupName.value = data.name
  } catch { /* Keep the group navigation available if its name cannot be loaded. */ }
})
const page = ref(1), pageSize = ref(20), loading = ref(false), error = ref('')
const keyword = ref(''), timeRange = ref([])
const resultVisible = ref(false), selectedRecord = ref(null)
const resultLanguage = computed(() => {
  const value = selectedRecord.value?.result
  if (value && typeof value === 'object') return 'json'
  try { JSON.parse(value); return 'json' } catch { return 'plaintext' }
})
async function copyResult(event) {
  const container = event?.currentTarget?.closest('.el-drawer') || document.body
  try {
    await copyText(formatResult(selectedRecord.value?.result), container)
    ElMessage.success('已复制结果内容')
  } catch { ElMessage.error('复制失败，请在代码视图中选中内容复制') }
}
const hasFilters = computed(() => Boolean(keyword.value.trim() || timeRange.value?.length))
let keywordTimer
let requestID = 0
function openResult(row) { selectedRecord.value = row; resultVisible.value = true }
function handleKeywordInput() { clearTimeout(keywordTimer); keywordTimer = setTimeout(searchRecords, 300) }
function searchRecords() { clearTimeout(keywordTimer); page.value = 1; loadRecords() }
function resetFilters() { keyword.value = ''; timeRange.value = []; searchRecords() }
async function loadRecords() {
  const currentRequest = ++requestID
  loading.value = true
  error.value = ''
  try {
    const params = { item_id: route.params.id, page: page.value, page_size: pageSize.value, keyword: keyword.value.trim() }
    if (timeRange.value?.length === 2) {
      params.start_time = new Date(timeRange.value[0].replace(' ', 'T')).toISOString()
      params.end_time = new Date(timeRange.value[1].replace(' ', 'T')).toISOString()
    }
    const { data } = await getInspectionRecords(params)
    if (currentRequest !== requestID) return
    item.value = data.item || {}
    records.value = data.list || []
    total.value = Number(data.total || 0)
    if (!records.value.length && total.value && page.value > 1) {
      page.value = Math.max(1, Math.ceil(total.value / pageSize.value))
      await loadRecords()
    }
  } catch {
    if (currentRequest !== requestID) return
    records.value = []
    total.value = 0
    error.value = '历史记录加载失败，请刷新页面重试'
  } finally {
    if (currentRequest === requestID) loading.value = false
  }
}
function changePageSize() { page.value = 1; loadRecords() }
function parseResult(value) { if (value && typeof value === 'object') return value; try { return JSON.parse(value || '{}') || {} } catch { return {} } }
function statusCode(value) { const result = parseResult(value); return result.status_code || result.response?.status_code || '—' }
function formatResult(value) { if (value && typeof value === 'object') return JSON.stringify(value, null, 2); try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value || '(无输出)' } }
function formatTime(value) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false }) }
watch(() => route.params.id, () => { clearTimeout(keywordTimer); page.value = 1; item.value = {}; records.value = []; keyword.value = ''; timeRange.value = []; resultVisible.value = false; loadRecords() }, { immediate: true })
onBeforeUnmount(() => { requestID++; clearTimeout(keywordTimer) })
</script>

<style scoped>
.history-page { display: flex; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 1 auto; grid-template-columns: minmax(160px, 190px) 250px max-content; gap: 8px; }
.filter-bar > * { min-width: 0; width: 100%; }
.filter-bar > .el-button { width: auto; justify-self: start; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.result-meta { margin-bottom: 20px; }
@media (max-width: 760px) {
  .task-toolbar { align-items: stretch; flex-direction: column; }
  .filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .task-search, .filter-bar :deep(.record-time-range-container) { grid-column: 1 / -1; }
  .page-header-actions { justify-content: flex-end; }
}
@media (min-width: 761px) and (max-width: 1180px) {
  .task-toolbar { align-items: stretch; flex-wrap: wrap; }
  .filter-bar { width: 100%; flex: 1 1 100%; }
  .page-header-actions { margin-left: auto; }
}
.result-code-heading { display: flex; align-items: center; justify-content: space-between; margin: 16px 0 10px; color: var(--ops-text); font-size: 13px; }
.history-pagination { display: flex; justify-content: flex-end; padding-top: 16px; overflow-x: auto; }
</style>

<template>
  <div class="page-container">
    <div class="rule-toolbar" role="search" aria-label="自动化任务执行记录筛选">
      <div class="filters">
        <el-input
          v-model="query.keyword"
          :prefix-icon="Search"
          clearable
          placeholder="搜索任务名称"
          @input="handleKeywordInput"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        />
        <el-select v-model="query.status" clearable placeholder="执行状态" @change="handleSearch">
          <el-option label="执行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-select v-model="query.trigger_type" clearable placeholder="触发方式" @change="handleSearch">
          <el-option label="手动执行" value="manual" />
          <el-option label="故障触发" value="fault" /><el-option label="拨测调用" value="probe" />
        </el-select>
        <RecordTimeRange v-model="dateRange" @change="handleSearch" />
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
      </div>
    </div>

    <el-card class="record-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="76" />
        <el-table-column prop="task_name" label="自动化任务" min-width="180" show-overflow-tooltip />
        <el-table-column label="触发方式" width="130">
          <template #default="{ row }">{{ triggerText(row) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="96">
          <template #default="{ row }">
            <el-tag :type="statusMeta(row.status).type" size="small">
              {{ statusMeta(row.status).text }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="退出码" width="86" align="center">
          <template #default="{ row }">{{ row.status === 'running' ? '—' : row.exit_code }}</template>
        </el-table-column>
        <el-table-column label="耗时" width="110">
          <template #default="{ row }">{{ formatDuration(row.duration_ms) }}</template>
        </el-table-column>
        <el-table-column label="执行输出" min-width="280">
          <template #default="{ row }">
            <span class="execution-output" :class="{ error: row.error }">
              {{ row.error || row.output || '—' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="执行时间" width="190">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>

      <div class="pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          layout="total, sizes, prev, pager, next"
          @current-change="fetchData"
          @size-change="changePageSize"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import RecordTimeRange from '../../components/RecordTimeRange.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Refresh, Search } from '@element-plus/icons-vue'
import { getAutomationExecutions } from '../../api/automationTask'

const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const loading = ref(false)
const tableData = ref([])
const dateRange = ref([])
const query = reactive({ keyword: '', status: '', trigger_type: '' })
const hasFilters = computed(() => Boolean(query.keyword || query.status || query.trigger_type || dateRange.value?.length))
let keywordSearchTimer

onMounted(fetchData)
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))

async function fetchData() {
  loading.value = true
  try {
    const response = await getAutomationExecutions({
      page: page.value,
      page_size: pageSize.value,
      keyword: query.keyword.trim(),
      status: query.status,
      trigger_type: query.trigger_type,
      start_time: dateRange.value?.[0],
      end_time: dateRange.value?.[1],
    })
    tableData.value = response.data.list || []
    total.value = response.data.total || 0
  } finally {
    loading.value = false
  }
}

function handleKeywordInput() {
  clearTimeout(keywordSearchTimer)
  keywordSearchTimer = setTimeout(handleSearch, 300)
}

function handleSearch() {
  clearTimeout(keywordSearchTimer)
  page.value = 1
  fetchData()
}

function clearFilters() {
  Object.assign(query, { keyword: '', status: '', trigger_type: '' })
  dateRange.value = []
  handleSearch()
}

function changePageSize() {
  page.value = 1
  fetchData()
}

function statusMeta(status) {
  return {
    success: { text: '成功', type: 'success' },
    failed: { text: '失败', type: 'danger' },
    running: { text: '执行中', type: 'primary' },
  }[status] || { text: status || '未知', type: 'info' }
}

function triggerText(row) {
  if (row.trigger_type === 'probe') return row.trigger_id ? `拨测调用 #${row.trigger_id}` : '拨测测试'
  if (row.trigger_type === 'manual') return '手动执行'
  if (row.trigger_type === 'fault') return row.trigger_id ? `故障触发 #${row.trigger_id}` : '故障触发'
  return row.trigger_type || '—'
}

function formatDuration(value) {
  const milliseconds = Number(value)
  if (!Number.isFinite(milliseconds)) return '—'
  if (milliseconds < 1000) return `${milliseconds} ms`
  return `${(milliseconds / 1000).toFixed(milliseconds < 10000 ? 1 : 0)} s`
}

function formatTime(value) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false })
}
</script>

<style scoped>
.page-container { --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.rule-toolbar, .filters, .toolbar-actions { display: flex; align-items: center; gap: 8px; }
.rule-toolbar { align-items: center; justify-content: space-between; }
.filters { min-width: 0; flex: 1; flex-wrap: wrap; }
.toolbar-actions { flex: 0 0 auto; }
.toolbar-actions :deep(.el-button + .el-button) { margin-left: 0; }
.filters .el-input { width: 180px; }
.filters .el-select { width: 120px; }
.record-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.record-card :deep(.el-card__body) { padding: 16px; }
.execution-output { display: block; overflow: hidden; color: var(--el-text-color-regular); font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; line-height: 20px; text-overflow: ellipsis; white-space: nowrap; }
.execution-output.error { color: var(--el-color-danger); }
.pagination { display: flex; justify-content: flex-end; margin-top: 16px; }
@media (max-width: 1100px) { .rule-toolbar { align-items: stretch; flex-direction: column; }.filters { flex-wrap: wrap; }.toolbar-actions { justify-content: flex-end; } }
@media (max-width: 680px) { .filters .el-input, .filters .el-select, .filters :deep(.el-date-editor) { width: 100%; }.filters :deep(.el-date-editor) { flex-basis: 100%; } }
</style>

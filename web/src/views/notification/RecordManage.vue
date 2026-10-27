<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="通知记录筛选">
        <el-input v-model="query.keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索通知记录" placeholder="搜索通知对象、标题或事件标识" @input="handleKeywordInput" @clear="search" />
        <el-select v-model="query.status" clearable aria-label="按发送状态筛选" placeholder="发送状态" @change="search"><el-option label="成功" value="success" /><el-option label="失败" value="failed" /></el-select>
        <el-select v-model="query.eventType" clearable aria-label="按事件类型筛选" placeholder="事件类型" @change="search"><el-option v-for="item in eventTypes" :key="item.value" :label="item.label" :value="item.value" /></el-select>
        <el-date-picker v-model="dateRange" type="datetimerange" value-format="YYYY-MM-DD HH:mm:ss" start-placeholder="开始时间" end-placeholder="结束时间" range-separator="至" @change="search" />
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions"><el-button :icon="Refresh" :loading="loading" @click="loadRecords">刷新</el-button></div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="records" v-loading="loading" row-key="id" stripe class="record-table">
        <el-table-column label="状态" width="76" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 'success' ? 'success' : 'danger'" size="small" effect="light">
              {{ row.status === 'success' ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="channel_name" label="通知对象" min-width="150" show-overflow-tooltip />
        <el-table-column label="事件类型" min-width="170" show-overflow-tooltip>
          <template #default="{ row }">{{ eventTypeText(row.event_type) }}</template>
        </el-table-column>
        <el-table-column prop="title" label="标题" min-width="240" show-overflow-tooltip />
        <el-table-column label="发送时间" width="160">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="60" fixed="right" align="center">
          <template #default="{ row }">
            <el-tooltip content="查看详情" placement="top"><el-button link type="primary" :icon="View" aria-label="查看通知详情" @click="openDetail(row)" /></el-tooltip>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>

      <div v-if="total > 0" class="task-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="loadRecords"
          @size-change="handlePageSizeChange"
        />
      </div>
    </el-card>

    <el-drawer v-model="detailVisible" class="record-drawer" size="680px" direction="rtl">
      <template #header>
        <div class="drawer-header">
          <DrawerBackButton @click="detailVisible = false" /><span class="drawer-title">通知详情</span>
        </div>
      </template>

      <div v-if="currentRecord" class="detail-body">
        <div class="detail-row">
          <span>通知对象</span>
          <strong>{{ currentRecord.channel_name || '-' }}</strong>
        </div>
        <div class="detail-row">
          <span>状态</span>
          <el-tag :type="currentRecord.status === 'success' ? 'success' : 'danger'" size="small" effect="light">
            {{ currentRecord.status === 'success' ? '成功' : '失败' }}
          </el-tag>
        </div>
        <div class="detail-row">
          <span>事件类型</span>
          <strong>{{ eventTypeText(currentRecord.event_type) }}</strong>
        </div>
        <div class="detail-row">
          <span>事件标识</span>
          <strong>{{ currentRecord.event_key || '-' }}</strong>
        </div>
        <div class="detail-row">
          <span>标题</span>
          <strong>{{ currentRecord.title || '-' }}</strong>
        </div>
        <div class="detail-row">
          <span>发送时间</span>
          <strong>{{ formatTime(currentRecord.created_at) }}</strong>
        </div>

        <div class="detail-section">
          <span class="section-title">请求内容</span>
          <pre>{{ formatJson(currentRecord.request_body) }}</pre>
        </div>
        <div class="detail-section">
          <span class="section-title">响应内容</span>
          <pre>{{ currentRecord.response || '-' }}</pre>
        </div>
        <div class="detail-section">
          <span class="section-title">错误信息</span>
          <pre>{{ currentRecord.error || '-' }}</pre>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Refresh, Search, View } from '@element-plus/icons-vue'
import { getNotificationRecords } from '../../api/notification'

const records = ref([])
const loading = ref(false)
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)
const detailVisible = ref(false)
const currentRecord = ref(null)
const dateRange = ref([])
const query = reactive({ keyword: '', status: '', eventType: '' })
const hasFilters = computed(() => query.keyword.trim() !== '' || query.status || query.eventType || dateRange.value?.length)
const eventTypes = [
  { value: 'alert.rule.firing', label: '告警规则触发' },
  { value: 'alert.rule.recovery', label: '告警规则恢复' },
  { value: 'alert.group', label: '告警分组通知' },
  { value: 'probe.task.failure', label: '拨测任务失败' },
  { value: 'probe.task.recovery', label: '拨测任务恢复' },
  { value: 'scheduled.task.firing', label: '历史定时任务告警' },
  { value: 'scheduled.task.recovery', label: '历史定时任务恢复' },
  { value: 'certificate.domain.firing', label: '证书域名告警' },
  { value: 'certificate.domain.recovery', label: '证书域名恢复' },
  { value: 'certificate.domain.alert', label: '证书通知' },
  { value: 'alert.action.failure', label: '处置执行失败' },
  { value: 'alert.action.success', label: '处置执行成功' },
  { value: 'notification.test', label: '通知测试' },
]
let keywordTimer

onMounted(loadRecords)
onBeforeUnmount(() => clearTimeout(keywordTimer))

async function loadRecords() {
  loading.value = true
  try {
    const res = await getNotificationRecords({
      page: page.value,
      page_size: pageSize.value,
      keyword: query.keyword.trim(),
      status: query.status,
      event_type: query.eventType,
      start_time: dateRange.value?.[0],
      end_time: dateRange.value?.[1],
    })
    records.value = res.data?.list || []
    total.value = res.data?.total || 0
    if (records.value.length === 0 && total.value > 0 && page.value > 1) {
      page.value -= 1
      await loadRecords()
    }
  } finally {
    loading.value = false
  }
}

function handleKeywordInput() {
  clearTimeout(keywordTimer)
  keywordTimer = setTimeout(search, 300)
}

function search() {
  clearTimeout(keywordTimer)
  page.value = 1
  loadRecords()
}

function clearFilters() {
  Object.assign(query, { keyword: '', status: '', eventType: '' })
  dateRange.value = []
  search()
}

function handlePageSizeChange() {
  page.value = 1
  loadRecords()
}

function openDetail(row) {
  currentRecord.value = row
  detailVisible.value = true
}

function eventTypeText(value) {
  return eventTypes.find(item => item.value === value)?.label || value || '-'
}

function formatJson(value) {
  if (!value) return '-'
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return value
  }
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}
</script>

<style scoped>
.task-page { --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 230px 105px 120px 250px auto; gap: 8px; }
.filter-bar > * { min-width: 0; width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }

:deep(.record-table.el-table) {
  --el-table-border-color: #edf0f5;
  --el-table-header-bg-color: #f7f9fc;
}

:deep(.record-table .el-table__header-wrapper th) {
  color: #5d6675;
  font-weight: 700;
}

:deep(.record-table .el-table__cell) {
  padding: 11px 0;
}

:deep(.record-table .el-table__row:hover > td.el-table__cell) {
  background: #f8fbff;
}

:deep(.record-table .el-tag) { border-radius: 0; }

:deep(.record-drawer.el-drawer) {
  border-radius: 0;
}

:deep(.record-drawer .el-drawer__header) {
  margin-bottom: 0;
  padding: 18px 20px;
  border-bottom: 1px solid #e5e9f2;
}

:deep(.record-drawer .el-drawer__body) {
  background: var(--ops-surface);
  padding: 18px 20px;
}

.drawer-header {
  display: flex;
  align-items: center;
  width: 100%;
}

.drawer-title {
  color: var(--ops-text);
  font-size: 16px;
  font-weight: 600;
}

.detail-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-row {
  display: grid;
  grid-template-columns: 92px 1fr;
  align-items: center;
  gap: 12px;
  color: var(--ops-text);
  font-size: 14px;
}

.detail-row span {
  color: #7f8998;
}

.detail-row strong {
  min-width: 0;
  overflow-wrap: anywhere;
  font-weight: 500;
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 4px;
}

.section-title {
  color: var(--ops-text);
  font-weight: 600;
}

pre {
  min-height: 44px;
  max-height: 260px;
  margin: 0;
  overflow: auto;
  padding: 12px;
  border: 1px solid #e5e8ef;
  border-radius: 6px;
  background: #f8fafc;
  color: #26313f;
  font-family: Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.55;
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 760px) {
  .task-toolbar { align-items: stretch; flex-direction: column; }
  .filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .task-search, .filter-bar :deep(.el-date-editor) { grid-column: 1 / -1; }
  .page-header-actions { justify-content: flex-end; }
  .task-pagination { overflow-x: auto; justify-content: flex-start; }

  .detail-row {
    grid-template-columns: 1fr;
    gap: 4px;
  }
}

@media (min-width: 761px) and (max-width: 1180px) { .task-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; } }
</style>

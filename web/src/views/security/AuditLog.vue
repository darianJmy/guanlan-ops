<template>
  <div class="audit-page">
    <header class="audit-toolbar">
      <div class="filter-bar" role="search" aria-label="审计日志筛选">
        <el-input v-model="query.keyword" class="audit-search" :prefix-icon="Search" clearable aria-label="搜索审计记录" placeholder="搜索操作人、资源或路径" @input="handleKeywordInput" @keyup.enter="search" @clear="search" />
        <el-select v-model="query.module" clearable filterable aria-label="按业务模块筛选" placeholder="业务模块" @change="search">
          <el-option v-for="item in options.modules" :key="item" :label="item" :value="item" />
        </el-select>
        <el-select v-model="query.action" clearable aria-label="按操作动作筛选" placeholder="操作动作" @change="search">
          <el-option v-for="item in options.actions" :key="item" :label="item" :value="item" />
        </el-select>
        <el-select v-model="query.result" clearable aria-label="按操作结果筛选" placeholder="操作结果" @change="search"><el-option label="成功" value="成功" /><el-option label="失败" value="失败" /></el-select>
        <el-date-picker v-model="dateRange" type="datetimerange" value-format="YYYY-MM-DD HH:mm:ss" start-placeholder="开始时间" end-placeholder="结束时间" range-separator="至" @change="search" />
        <el-button :disabled="!hasFilters" @click="resetFilters">清空</el-button>
      </div>
      <div class="page-header-actions"><el-button :icon="Refresh" :loading="loading" @click="refreshAll">刷新</el-button></div>
    </header>

    <el-card class="audit-card" shadow="never">
      <el-table v-loading="loading" :data="records" row-key="id" stripe class="audit-table">
        <el-table-column label="操作时间" width="150"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
        <el-table-column label="操作人" width="90" show-overflow-tooltip><template #default="{ row }"><strong class="operator-name">{{ row.username || '未知用户' }}</strong></template></el-table-column>
        <el-table-column prop="module" label="业务模块" min-width="100" show-overflow-tooltip />
        <el-table-column label="操作动作" width="90"><template #default="{ row }"><el-tag size="small" effect="light" :type="actionType(row.action)">{{ row.action }}</el-tag></template></el-table-column>
        <el-table-column label="操作对象" min-width="160" show-overflow-tooltip><template #default="{ row }"><div class="resource"><strong>{{ row.resource_name || resourceText(row.resource_type) }}</strong><small>{{ row.resource_id ? `ID: ${row.resource_id}` : row.path }}</small></div></template></el-table-column>
        <el-table-column label="结果" width="70"><template #default="{ row }"><el-tag size="small" effect="light" :type="row.result === '成功' ? 'success' : 'danger'">{{ row.result }}</el-tag></template></el-table-column>
        <el-table-column prop="ip" label="来源 IP" width="115" show-overflow-tooltip />
        <el-table-column label="耗时" width="70"><template #default="{ row }">{{ row.duration_ms }} ms</template></el-table-column>
        <el-table-column label="操作" width="60" fixed="right" align="center"><template #default="{ row }"><el-tooltip content="查看详情" placement="top"><el-button link type="primary" :icon="View" aria-label="查看审计详情" @click="openDetail(row)" /></el-tooltip></template></el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>

      <div v-if="total > 0" class="pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @current-change="loadRecords" @size-change="changePageSize" /></div>
    </el-card>

    <el-drawer v-model="detailVisible" title="操作审计详情" size="640px" class="audit-drawer">
      <template #header><div class="standard-drawer-header"><DrawerBackButton @click="detailVisible = false" /><span class="standard-drawer-title">操作审计详情</span></div></template>
      <div v-if="current" class="detail-body">
        <div class="detail-status"><div><span>{{ current.action }}</span><strong>{{ current.module }} · {{ current.resource_name || resourceText(current.resource_type) }}</strong></div><el-tag :type="current.result === '成功' ? 'success' : 'danger'">{{ current.result }}</el-tag></div>
        <dl>
          <div><dt>操作人</dt><dd>{{ current.username || '-' }}</dd></div><div><dt>操作时间</dt><dd>{{ formatTime(current.created_at) }}</dd></div>
          <div><dt>请求方法</dt><dd><code>{{ current.method }}</code></dd></div><div><dt>状态码</dt><dd>{{ current.status_code }}</dd></div>
          <div><dt>来源 IP</dt><dd>{{ current.ip || '-' }}</dd></div><div><dt>执行耗时</dt><dd>{{ current.duration_ms }} ms</dd></div>
          <div class="wide"><dt>请求路径</dt><dd><code>{{ current.path }}</code></dd></div>
          <div class="wide"><dt>结果说明</dt><dd>{{ current.response_message || (current.result === '成功' ? '操作成功' : '操作失败') }}</dd></div>
          <div class="wide"><dt>User-Agent</dt><dd>{{ current.user_agent || '-' }}</dd></div>
        </dl>
        <section class="request-summary"><div><strong>脱敏请求摘要</strong><span>密码、Token、密钥和认证配置已自动隐藏</span></div><pre>{{ formatSummary(current.request_summary) }}</pre></section>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Refresh, Search, View } from '@element-plus/icons-vue'
import { getAuditLogs, getAuditLogStats } from '../../api/audit'

const records = ref([]), loading = ref(false), page = ref(1), pageSize = ref(20), total = ref(0)
const dateRange = ref([]), detailVisible = ref(false), current = ref(null)
const options = reactive({ modules: [], actions: [] })
const query = reactive({ keyword: '', module: '', action: '', result: '' })
const hasFilters = computed(() => query.keyword.trim() !== '' || query.module || query.action || query.result || dateRange.value?.length)
let keywordSearchTimer

onMounted(refreshAll)
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))

async function refreshAll() {
  const [optionsResult] = await Promise.allSettled([loadOptions(), loadRecords()])
  if (optionsResult.status === 'rejected') Object.assign(options, { modules: [], actions: [] })
}

async function loadOptions() {
  const response = await getAuditLogStats()
  options.modules = response.data?.modules || []
  options.actions = response.data?.actions || []
}

async function loadRecords() {
  loading.value = true
  try {
    const response = await getAuditLogs({ page: page.value, page_size: pageSize.value, keyword: query.keyword.trim(), module: query.module, action: query.action, result: query.result, start_time: dateRange.value?.[0], end_time: dateRange.value?.[1] })
    records.value = response.data?.list || []
    total.value = response.data?.total || 0
  } finally { loading.value = false }
}

function search() { clearTimeout(keywordSearchTimer); page.value = 1; loadRecords() }
function handleKeywordInput() { clearTimeout(keywordSearchTimer); keywordSearchTimer = setTimeout(search, 300) }
function resetFilters() { Object.assign(query, { keyword: '', module: '', action: '', result: '' }); dateRange.value = []; search() }
function changePageSize() { page.value = 1; loadRecords() }
function openDetail(row) { current.value = row; detailVisible.value = true }
function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-' }
function actionType(value) { return ({ 删除: 'danger', 执行: 'warning', 发布: 'warning', 登录: 'info', 查看: 'info', 下载: 'info' })[value] || 'primary' }
function resourceText(value) { return ({ user: '用户', role: '角色', datasource: '数据源', alert_rule: '告警规则', alert_rule_group: '规则组', fault_center: '故障中心', session: '登录会话' })[value] || value || '平台资源' }
function formatSummary(value) { if (!value) return '该操作没有请求参数'; try { return JSON.stringify(JSON.parse(value), null, 2) } catch { return value } }
</script>

<style scoped>
.audit-page { --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.audit-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 180px 110px 100px 100px 250px auto; gap: 8px; }
.filter-bar > * { min-width: 0; width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.audit-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.audit-card :deep(.el-card__body) { padding: 16px; }
.audit-table :deep(th.el-table__cell) { color: var(--ops-muted); background: var(--ops-surface-soft); font-weight: 600; }
.audit-table :deep(.el-tag) { border-radius: 0; }
.operator-name, .resource strong { color: #344054; font-size: 12px; font-weight: 600; }
.resource { display: flex; min-width: 0; flex-direction: column; gap: 4px; }.resource small { overflow: hidden; color: var(--ops-subtle); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.pagination { display: flex; justify-content: flex-end; margin-top: 16px; }
.detail-body { display: flex; flex-direction: column; gap: 20px; }.detail-status { display: flex; align-items: center; justify-content: space-between; padding: 16px; border: 1px solid #e5e9f0; border-radius: 8px; background: #f8fafc; }.detail-status > div { display: flex; flex-direction: column; gap: 5px; }.detail-status span { color: #8a96a8; font-size: 11px; }.detail-status strong { color: #344054; font-size: 15px; }dl { display: grid; grid-template-columns: 1fr 1fr; gap: 14px 20px; margin: 0; }dl > div { display: flex; min-width: 0; flex-direction: column; gap: 5px; }.wide { grid-column: 1 / -1; }dt { color: var(--ops-subtle); font-size: 11px; }dd { margin: 0; color: #344054; font-size: 13px; overflow-wrap: anywhere; }code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }.request-summary { display: flex; flex-direction: column; gap: 10px; }.request-summary > div { display: flex; align-items: baseline; gap: 10px; }.request-summary strong { color: #344054; font-size: 13px; }.request-summary span { color: var(--ops-subtle); font-size: 10px; }.request-summary pre { min-height: 100px; max-height: 360px; margin: 0; overflow: auto; padding: 14px; border: 1px solid #e5e9f0; border-radius: 8px; background: #f7f9fc; color: #475467; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; line-height: 1.65; white-space: pre-wrap; }
@media (max-width: 1180px) { .audit-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; } }
@media (max-width: 760px) { .audit-toolbar { flex-direction: column; }.filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }.audit-search, .filter-bar :deep(.el-date-editor) { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; }.pagination { overflow-x: auto; justify-content: flex-start; }dl { grid-template-columns: 1fr; }.wide { grid-column: auto; } }
</style>

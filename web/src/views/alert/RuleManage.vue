<template>
  <div class="rule-page">
    <header class="detail-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="router.push('/alert-rules')">返回</el-button>
      <div class="group-summary">
        <div class="group-name">
          <template v-if="nameEditing">
            <el-input
              ref="nameInputRef"
              v-model="nameDraft"
              class="group-name-input"
              size="small"
              maxlength="128"
              aria-label="规则组名称"
              @keyup.enter="saveGroupName"
              @keyup.esc="cancelNameEdit"
            />
            <el-tooltip content="确认" placement="top">
              <el-button text circle class="name-action is-confirm" :icon="Check" :loading="nameSaving" aria-label="保存规则组名称" @click="saveGroupName" />
            </el-tooltip>
            <el-tooltip content="取消" placement="top">
              <el-button text circle class="name-action is-cancel" :icon="Close" :disabled="nameSaving" aria-label="取消修改规则组名称" @click="cancelNameEdit" />
            </el-tooltip>
          </template>
          <template v-else>
            <p>{{ group.name || '规则组' }}</p>
            <el-tooltip content="编辑名称" placement="top">
              <el-button text circle :icon="Edit" aria-label="编辑规则组名称" @click="startNameEdit" />
            </el-tooltip>
          </template>
        </div>
      </div>
    </header>

    <section class="table-card">
      <div class="rule-toolbar" role="search" aria-label="告警规则筛选">
        <div class="filters">
          <el-input v-model="query.keyword" :prefix-icon="Search" clearable placeholder="搜索规则名称或 PromQL" @input="handleKeywordInput" @keyup.enter="handleSearch" @clear="handleSearch" />
          <el-select v-model="query.status" clearable placeholder="状态" @change="handleSearch">
            <el-option label="启用" value="enabled" />
            <el-option label="禁用" value="disabled" />
          </el-select>
          <el-button :disabled="!query.keyword && !query.status" @click="clearFilters">清空</el-button>
        </div>
        <div class="toolbar-actions">
          <el-dropdown trigger="click" :disabled="!!batchAction" @command="handleBatchCommand">
            <el-button :disabled="!!batchAction" :loading="!!batchAction">
              批量操作
              <el-icon class="batch-operation-arrow"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="enable" :disabled="!selectedRows.length" v-permission="['alert:rule:edit']">批量启用</el-dropdown-item>
                <el-dropdown-item command="disable" :disabled="!selectedRows.length" v-permission="['alert:rule:edit']">批量禁用</el-dropdown-item>
                <el-dropdown-item command="delete" :disabled="!selectedRows.length" v-permission="['alert:rule:delete']">批量删除</el-dropdown-item>
                <el-dropdown-item command="import" divided :icon="Upload" v-permission="['alert:rule:create']">导入配置</el-dropdown-item>
                <el-dropdown-item command="export" :icon="Download">导出配置</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button :icon="Refresh" :loading="loading" @click="loadRules">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="router.push(`/alert-rules/${groupId}/create`)" v-permission="['alert:rule:create']">创建</el-button>
        </div>
      </div>
      <el-table v-loading="loading" :data="rules" row-key="id" @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column label="规则名称" min-width="190">
          <template #default="{ row }">
            <span class="rule-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="告警等级" width="150">
          <template #default="{ row }">
            <div class="severity-list">
              <el-tag v-for="severity in ruleSeverities(row)" :key="severity" :type="severityTagType(severity)" size="small">{{ severity }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="数据源" min-width="320">
          <template #default="{ row }">
            <div class="tag-list">
              <el-tag v-for="name in row.datasource_group_names" :key="`group-${name}`" size="small" effect="plain">{{ name }}</el-tag>
              <el-tag v-for="name in row.datasource_names" :key="`source-${name}`" size="small" effect="plain">{{ name }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="fault_center_name" label="故障中心" min-width="140" />
        <el-table-column label="启用状态" width="110">
          <template #default="{ row }">
            <el-switch v-model="row.status" :active-value="1" :inactive-value="0" :loading="togglingId === row.id" :before-change="() => toggleRuleStatus(row)" aria-label="切换告警规则状态" v-permission="['alert:rule:edit']" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" aria-label="编辑告警规则" title="编辑" @click="router.push(`/alert-rules/${groupId}/${row.id}/edit`)" v-permission="['alert:rule:edit']" />
            <el-button link type="danger" :icon="Delete" aria-label="删除告警规则" title="删除" @click="removeRule(row)" v-permission="['alert:rule:delete']" />
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          layout="total, sizes, prev, pager, next"
          @current-change="loadRules"
          @size-change="handleSizeChange"
        />
      </div>
    </section>

    <el-dialog v-model="importVisible" class="rule-import-dialog" title="导入告警规则" width="min(760px, 94vw)" top="5vh" :close-on-click-modal="false" @closed="resetImportDialog">
      <section class="import-note" role="note">
        <strong>YAML 配置</strong>
        <span>配置将导入当前规则组“{{ group.name || '规则组' }}”。系统按规则名称识别重复项，不会导入或删除告警历史。</span>
      </section>
      <div class="import-actions">
        <el-upload :auto-upload="false" :show-file-list="false" accept=".yaml,.yml,application/yaml,text/yaml,text/x-yaml" :on-change="readImportFile">
          <el-button>选择配置文件</el-button>
        </el-upload>
        <span>也可以直接在下方粘贴并编辑配置</span>
      </div>
      <el-input v-model="importContent" type="textarea" :rows="14" class="import-editor" :placeholder="importExample" @input="clearImportPreview" />
      <div class="import-policy">
        <div><strong>覆盖同名规则</strong><span>关闭时跳过同名规则；开启后更新规则配置，保留原规则 ID 和告警历史。</span></div>
        <el-switch v-model="importOverwrite" @change="clearImportPreview" />
      </div>
      <div v-if="importPreview" class="import-preview" role="status">
        <span>共 {{ importPreview.total }} 条</span><strong class="created">新增 {{ importPreview.created }}</strong><strong class="updated">覆盖 {{ importPreview.updated }}</strong><strong>跳过 {{ importPreview.skipped }}</strong>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="importVisible = false">取消</el-button>
          <el-button :loading="validatingImport" :disabled="!importContent.trim()" @click="validateImportConfig">校验配置</el-button>
          <el-button type="primary" :loading="importing" :disabled="!importPreview" @click="confirmImportConfig">确认导入</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, ArrowLeft, Check, Close, Delete, Download, Edit, Plus, Refresh, Search, Upload } from '@element-plus/icons-vue'
import { batchDeleteAlertRules, batchUpdateAlertRuleStatus, deleteAlertRule, exportAlertRules, getAlertRuleGroup, getAlertRules, importAlertRules, updateAlertRuleGroup, updateAlertRuleStatus } from '../../api/alertRule'

const rules = ref([])
const route = useRoute(), router = useRouter(), groupId = Number(route.params.groupId), group = reactive({ name: '', description: '' })
const loading = ref(false), togglingId = ref(null)
const selectedRows = ref([]), batchAction = ref('')
const importVisible = ref(false), importContent = ref(''), importOverwrite = ref(false), importPreview = ref(null), validatingImport = ref(false), importing = ref(false)
const nameEditing = ref(false), nameDraft = ref(''), nameSaving = ref(false), nameInputRef = ref()
const page = ref(1), pageSize = ref(10), total = ref(0)
const query = reactive({ keyword: '', status: '' })
const severityOrder = { P0: 0, P1: 1, P2: 2 }
const importExample = `apiVersion: aiops/v1
kind: AlertRuleList
rules:
  - name: CPU 使用率过高
    expression: 100 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[1m])) * 100
    datasource_ids:
      - 1
    fault_center_id: 1
    for_seconds: 120
    recovery_wait_seconds: 0
    evaluation_interval_seconds: 60
    external_labels:
      env: production
    annotations:
      description: 实例 CPU 使用率持续过高
    conditions:
      - severity: P0
        operator: ">"
        threshold: 90
    status: 1`
let keywordSearchTimer
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))
onMounted(async () => {
  const [groupResult] = await Promise.allSettled([getAlertRuleGroup(groupId)])
  if (groupResult.status === 'fulfilled') Object.assign(group, groupResult.value.data || {})
  else { ElMessage.error('规则组不存在'); router.replace('/alert-rules'); return }
  await loadRules()
})

async function loadRules() {
  loading.value = true
  try {
    const data = (await getAlertRules({ group_id: groupId, keyword: query.keyword.trim(), status: query.status, page: page.value, page_size: pageSize.value })).data || {}
    rules.value = data.list || []
    selectedRows.value = []
    total.value = data.total || 0
  }
  finally { loading.value = false }
}

function handleKeywordInput() {
  clearTimeout(keywordSearchTimer)
  keywordSearchTimer = setTimeout(handleSearch, 300)
}
function handleSearch() {
  clearTimeout(keywordSearchTimer)
  page.value = 1
  loadRules()
}
function clearFilters() {
  Object.assign(query, { keyword: '', status: '' })
  handleSearch()
}
function handleSizeChange() { page.value = 1; loadRules() }
function handleSelectionChange(rows) { selectedRows.value = rows }
function handleBatchCommand(command) {
  if (command === 'enable') return handleBatchStatus(1)
  if (command === 'disable') return handleBatchStatus(0)
  if (command === 'delete') return handleBatchDelete()
  if (command === 'import') return openImportDialog()
  if (command === 'export') return downloadRuleConfig()
}
function openImportDialog() { resetImportDialog(); importVisible.value = true }
function resetImportDialog() { importContent.value = ''; importOverwrite.value = false; importPreview.value = null }
function clearImportPreview() { importPreview.value = null }
async function readImportFile(file) {
  const rawFile = file?.raw
  if (!rawFile) return
  if (rawFile.size > 2 * 1024 * 1024) return ElMessage.warning('配置文件不能超过 2 MB')
  importContent.value = await rawFile.text()
  clearImportPreview()
}
async function validateImportConfig() {
  validatingImport.value = true
  try {
    const response = await importAlertRules(groupId, importContent.value, importOverwrite.value, true)
    importPreview.value = response.data
    ElMessage.success('配置校验通过')
  } finally { validatingImport.value = false }
}
async function confirmImportConfig() {
  if (!importPreview.value) return
  const summary = importPreview.value
  const confirmed = await ElMessageBox.confirm(
    `确认导入 ${summary.total} 条告警规则吗？将新增 ${summary.created} 条、覆盖 ${summary.updated} 条、跳过 ${summary.skipped} 条。`,
    '确认导入',
    { type: summary.updated ? 'warning' : 'info', confirmButtonText: '导入', cancelButtonText: '取消' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return
  importing.value = true
  try {
    const response = await importAlertRules(groupId, importContent.value, importOverwrite.value, false)
    const result = response.data
    ElMessage.success(`导入完成：新增 ${result.created} 条，覆盖 ${result.updated} 条，跳过 ${result.skipped} 条`)
    importVisible.value = false
    page.value = 1
    await loadRules()
  } finally { importing.value = false }
}
async function downloadRuleConfig() {
  const response = await exportAlertRules(groupId)
  const blob = response.data instanceof Blob ? response.data : new Blob([response.data])
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = downloadFilename(response.headers?.['content-disposition']) || `alert-rules-${groupId}.yaml`
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(url)
}
function downloadFilename(disposition) {
  const match = String(disposition || '').match(/filename="?([^";]+)"?/i)
  return match?.[1] || ''
}
async function handleBatchStatus(status) {
  if (!selectedRows.value.length) return
  const count = selectedRows.value.length
  batchAction.value = status === 1 ? 'enable' : 'disable'
  try {
    await batchUpdateAlertRuleStatus(selectedRows.value.map(row => row.id), status)
    ElMessage.success(`已批量${status === 1 ? '启用' : '禁用'} ${count} 条告警规则`)
    await loadRules()
  } finally { batchAction.value = '' }
}
async function handleBatchDelete() {
  if (!selectedRows.value.length) return
  const count = selectedRows.value.length
  const removeCurrentPage = count === rules.value.length && page.value > 1
  const confirmed = await ElMessageBox.confirm(
    `确定删除选中的 ${count} 条告警规则吗？这些规则的活动告警将转为已恢复，删除后无法恢复。`,
    '批量删除告警规则',
    { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return

  batchAction.value = 'delete'
  try {
    await batchDeleteAlertRules(selectedRows.value.map(row => row.id))
    ElMessage.success(`已删除 ${count} 条告警规则`)
    if (removeCurrentPage) page.value -= 1
    await loadRules()
  } finally { batchAction.value = '' }
}
function ruleSeverities(row) {
  const values = row.conditions?.length ? row.conditions.map(item => item.severity) : [row.severity]
  return [...new Set(values.filter(Boolean))].sort((a, b) => (severityOrder[a] ?? 99) - (severityOrder[b] ?? 99))
}
function severityTagType(severity) { return severity === 'P0' ? 'danger' : severity === 'P1' ? 'warning' : severity === 'P2' ? 'primary' : 'info' }

function startNameEdit() {
  nameDraft.value = group.name || ''
  nameEditing.value = true
  nextTick(() => nameInputRef.value?.focus())
}

function cancelNameEdit() {
  if (nameSaving.value) return
  nameDraft.value = group.name || ''
  nameEditing.value = false
}

async function saveGroupName() {
  if (nameSaving.value) return
  const name = nameDraft.value.trim()
  if (!name) return ElMessage.warning('规则组名称不能为空')
  if (name === group.name) return cancelNameEdit()
  nameSaving.value = true
  try {
    await updateAlertRuleGroup(groupId, { name, description: group.description || '' })
    group.name = name
    nameEditing.value = false
    ElMessage.success('规则组名称已更新')
  } finally {
    nameSaving.value = false
  }
}

async function toggleRuleStatus(row) {
  const status = row.status === 1 ? 0 : 1
  togglingId.value = row.id
  try {
    await updateAlertRuleStatus(row.id, status)
    ElMessage.success(status === 1 ? '告警规则已启用' : '告警规则已停用')
    return true
  } catch {
    return false
  } finally {
    togglingId.value = null
  }
}

async function removeRule(row) {
  await ElMessageBox.confirm(`删除规则“${row.name}”后，其活动告警将转为已恢复。`, '删除告警规则', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  await deleteAlertRule(row.id)
  ElMessage.success('告警规则已删除')
  if (rules.value.length === 1 && page.value > 1) page.value -= 1
  await loadRules()
}

</script>

<style scoped>
.rule-page { display: flex; flex-direction: column; gap: 16px; }
.detail-header { display: flex; min-height: 32px; align-items: center; gap: 14px; }
.group-summary { min-width: 0; flex: 1; }
.group-name { display: flex; min-width: 0; align-items: center; gap: 4px; }
.group-name p { margin: 0; overflow: hidden; color: var(--ops-muted); font-size: 12px; font-weight: 400; line-height: 1.5; text-overflow: ellipsis; white-space: nowrap; }
.group-name-input { width: 140px; max-width: min(140px, calc(100vw - 210px)); }
.group-name-input :deep(.el-input__wrapper) { box-shadow: 0 0 0 1px var(--ops-primary, var(--ops-primary)) inset; }
.group-name .el-button { width: 28px; height: 28px; flex: 0 0 auto; color: var(--ops-muted); }
.group-name .el-button:hover { color: var(--ops-primary); background: var(--ops-primary-soft); }
.group-name .name-action.is-confirm { color: #1a8f68; }
.group-name .name-action.is-confirm:hover { color: #137555; background: #ebf8f3; }
.group-name .name-action.is-cancel { color: #c55252; }
.group-name .name-action.is-cancel:hover { color: #a83f3f; background: #fff0f0; }
.rule-toolbar, .filters, .toolbar-actions { display: flex; align-items: center; gap: 8px; }
.rule-toolbar { justify-content: space-between; margin-bottom: 16px; }
.toolbar-actions :deep(.el-button + .el-button) { margin-left: 0; }
.batch-operation-arrow { margin-left: 6px; }
.rule-import-dialog { display: flex; max-height: 90vh; flex-direction: column; }
.rule-import-dialog :deep(.el-dialog__body) { min-height: 0; overflow-y: auto; }
.import-note { display: flex; align-items: flex-start; gap: 10px; margin-bottom: 14px; padding: 11px 13px; color: #637083; background: #f3f8ff; border-left: 3px solid var(--ops-primary); font-size: 12px; line-height: 1.6; }
.import-note strong { flex: 0 0 auto; color: #315781; }
.import-actions { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; color: #8a96a8; font-size: 12px; }
.import-editor :deep(textarea) { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; line-height: 1.55; }
.import-policy { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-top: 14px; padding: 12px 14px; border: 1px solid #e1e8f1; border-radius: 7px; background: #fafbfd; }
.import-policy > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.import-policy strong { color: #344054; font-size: 13px; }
.import-policy span { color: #8a96a8; font-size: 11px; line-height: 1.5; }
.import-preview { display: flex; align-items: center; flex-wrap: wrap; gap: 14px; margin-top: 12px; padding: 10px 12px; color: var(--ops-muted); background: #f7f9fc; font-size: 12px; }
.import-preview strong { color: var(--ops-muted); }
.import-preview .created { color: #2f855a; }
.import-preview .updated { color: #b7791f; }
.dialog-footer { display: flex; justify-content: flex-end; gap: 8px; }
.table-card { padding: 16px; border: 1px solid #e5e9f0; border-radius: 8px; background: var(--ops-surface); }
.pagination-wrap { display: flex; justify-content: flex-end; margin-top: 16px; }
.filters .el-input { width: 280px; }.filters .el-select { width: 130px; }
.rule-name { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.tag-list { display: flex; align-items: center; flex-wrap: wrap; gap: 7px; }
.tag-list :deep(.el-tag) { border-radius: 0; }
.severity-list { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
.severity-list :deep(.el-tag) { min-width: 36px; justify-content: center; border-radius: 0; font-weight: 600; }
@media (max-width: 900px) { .rule-toolbar { align-items: stretch; flex-direction: column; }.filters { flex-wrap: wrap; }.filters .el-input { width: min(100%, 300px); }.toolbar-actions { justify-content: flex-end; }.pagination-wrap { overflow-x: auto; justify-content:flex-start; } }
</style>

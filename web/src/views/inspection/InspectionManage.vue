<template>
  <div class="task-page">

    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="拨测任务搜索">
        <el-input v-model="filters.keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索拨测任务" placeholder="搜索任务名称" @input="handleKeywordInput" @keyup.enter="handleSearch" @clear="handleSearch" />
        <el-select v-model="filters.type" class="filter-select filter-type" clearable aria-label="按拨测类型筛选" placeholder="类型" @change="handleSearch">
          <el-option v-for="item in taskTypes" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-select v-model="filters.status" class="filter-select filter-status" clearable aria-label="按状态筛选" placeholder="状态" @change="handleSearch">
          <el-option label="启用" value="enabled" />
          <el-option label="禁用" value="disabled" />
        </el-select>
        <el-button :disabled="!filters.keyword && !filters.type && !filters.status" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions">
        <el-dropdown trigger="click" :disabled="!!batchAction" @command="handleBatchCommand">
          <el-button :disabled="!!batchAction" :loading="!!batchAction">
            批量操作
            <el-icon class="batch-operation-arrow"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="enable" :disabled="!selectedRows.length" v-permission="['inspection:edit']">批量启用</el-dropdown-item>
              <el-dropdown-item command="disable" :disabled="!selectedRows.length" v-permission="['inspection:edit']">批量禁用</el-dropdown-item>
              <el-dropdown-item command="delete" :disabled="!selectedRows.length" v-permission="['inspection:delete']">批量删除</el-dropdown-item>
              <el-dropdown-item command="import" divided :icon="Upload" v-permission="['inspection:create']">导入配置</el-dropdown-item>
              <el-dropdown-item command="export" :icon="Download">导出配置</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="handleAdd" v-permission="['inspection:create']">创建</el-button>
      </div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" row-key="id" stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="name" label="任务名称" min-width="180">
          <template #default="{ row }"><div class="task-name"><button type="button" class="task-name-link" @click="handleDetail(row)">{{ row.name }}</button><small>{{ row.target || '未配置目标' }}</small></div></template>
        </el-table-column>
        <el-table-column label="类型" width="100">
          <template #default="{ row }"><span class="task-type-text">{{ typeMeta(row.type).label }}</span></template>
        </el-table-column>
        <el-table-column label="最近状态" width="120" align="center">
          <template #default="{ row }"><div class="node-state-list"><el-tag v-for="state in nodeStateSummary(row)" :key="state.key" class="node-state-tag" :type="state.type" size="small">{{ state.label }}</el-tag></div></template>
        </el-table-column>
        <el-table-column label="告警等级" width="100" align="center"><template #default="{ row }"><el-tag class="severity-tag" :type="severityTagType(row.severity)" size="small">{{ row.severity || 'P2' }}</el-tag></template></el-table-column>
        <el-table-column label="执行节点" min-width="210">
          <template #default="{ row }"><div class="node-tags"><el-tag v-for="node in executionNodes(row)" :key="node.node_key || `${node.node_type}:${node.node_id}`" class="execution-node-tag" size="small" effect="plain">{{ node.node_name }}</el-tag></div></template>
        </el-table-column>
        <el-table-column label="故障中心" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ faultCenterName(row.fault_center_id) }}</template></el-table-column>
        <el-table-column label="启用状态" width="100" align="center">
          <template #default="{ row }"><el-switch v-model="row.status" :active-value="1" :inactive-value="0" :loading="togglingId === row.id" :before-change="() => toggleTaskStatus(row)" :aria-label="`${row.name}启用状态`" v-permission="['inspection:edit']" /></template>
        </el-table-column>
        <el-table-column label="操作" width="132" align="center" fixed="right">
          <template #default="{ row }">
            <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑拨测任务" @click="handleEdit(row)" v-permission="['inspection:edit']" /></el-tooltip>
            <el-tooltip content="复制" placement="top"><el-button link type="primary" :icon="CopyDocument" aria-label="复制拨测任务" @click="handleCopy(row)" v-permission="['inspection:create']" /></el-tooltip>
            <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除拨测任务" @click="handleDelete(row)" v-permission="['inspection:delete']" /></el-tooltip>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div v-if="total > 0" class="task-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @current-change="loadTaskPage"
          @size-change="handlePageSizeChange"
        />
      </div>
    </el-card>

    <el-dialog v-model="importVisible" class="inspection-import-dialog" title="导入拨测任务" width="min(760px, 94vw)" top="5vh" :close-on-click-modal="false" @closed="resetImportDialog">
      <section class="import-note" role="note">
        <strong>YAML 配置</strong>
        <span>导入到当前规则组，按组内任务名称识别重复项。配置仅更新任务本身，不会导入或删除历史拨测记录；导出内容可能包含请求认证信息，请妥善保管。</span>
      </section>
      <div class="import-actions">
        <el-upload :auto-upload="false" :show-file-list="false" accept=".yaml,.yml,application/yaml,text/yaml,text/x-yaml" :on-change="readImportFile">
          <el-button>选择配置文件</el-button>
        </el-upload>
        <span>也可以直接在下方粘贴并编辑配置</span>
      </div>
      <el-input v-model="importContent" type="textarea" :rows="14" class="import-editor" :placeholder="importExample" @input="clearImportPreview" />
      <div class="import-policy">
        <div><strong>覆盖同名任务</strong><span>关闭时跳过同名任务；开启后按导入配置更新，保留原任务 ID 和历史记录。</span></div>
        <el-switch v-model="importOverwrite" @change="clearImportPreview" />
      </div>
      <div v-if="importPreview" class="import-preview" role="status">
        <span>共 {{ importPreview.total }} 条</span><strong class="created">新增 {{ importPreview.created }}</strong><strong class="updated">覆盖 {{ importPreview.updated }}</strong><strong>跳过 {{ importPreview.skipped }}</strong>
      </div>
      <template #footer>
        <div class="drawer-footer">
          <el-button @click="importVisible = false">取消</el-button>
          <el-button :loading="validatingImport" :disabled="!importContent.trim()" @click="validateImportConfig">校验配置</el-button>
          <el-button type="primary" :loading="importing" :disabled="!importPreview" @click="confirmImportConfig">确认导入</el-button>
        </div>
      </template>
    </el-dialog>

    <el-drawer v-model="drawerVisible" class="probe-task-drawer" :title="isEdit ? '编辑拨测任务' : '新建拨测任务'" size="min(460px, 94vw)" :close-on-click-modal="false">
      <template #header><div class="standard-drawer-header"><DrawerBackButton @click="drawerVisible = false" /><span class="standard-drawer-title">{{ isEdit ? '编辑拨测任务' : '新建拨测任务' }}</span></div></template>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="task-form">
        <section class="form-section">
          <div class="section-title"><strong>基本配置</strong><span>定义任务名称和拨测类型</span></div>
          <el-form-item label="任务名称" prop="name"><el-input v-model="form.name" maxlength="100" show-word-limit placeholder="例如：生产环境登录接口" /></el-form-item>
          <div class="probe-types" role="radiogroup" aria-label="拨测类型">
            <button v-for="item in taskTypes" :key="item.value" type="button" role="radio" :aria-checked="form.type === item.value" class="type-card" :class="{ active: form.type === item.value }" @click="changeType(item.value)"><span>{{ item.label }}</span><strong>{{ item.title }}</strong><small>{{ item.description }}</small></button>
          </div>
        </section>

        <section class="form-section">
          <div class="section-title"><strong>{{ currentType.title }}配置</strong><span>{{ currentType.hint }}</span></div>
          <div v-if="form.type === 'http'" class="form-grid">
            <el-form-item class="full-row method-field" label="请求方式"><el-radio-group v-model="form.method"><el-radio-button v-for="method in httpMethods" :key="method" :value="method">{{ method }}</el-radio-button></el-radio-group></el-form-item>
            <el-form-item class="full-row" label="请求 URL" prop="target"><el-input v-model="form.target" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" placeholder="https://api.example.com/health" /></el-form-item>
            <el-form-item label="请求超时"><el-input-number v-model="form.timeout_ms" :min="1000" :max="60000" :step="1000" /><span class="field-unit">毫秒</span></el-form-item>
            <el-form-item label="成功状态码"><el-input v-model="form.success_codes" placeholder="200-399" /></el-form-item>
            <el-form-item v-if="requestBodyVisible" class="full-row" label="请求 Body"><el-input v-model="form.body" type="textarea" :rows="4" class="mono-input" placeholder='{"key":"value"}' /></el-form-item>
            <el-collapse class="advanced-collapse full-row">
              <el-collapse-item name="http-advanced">
                <template #title><span class="advanced-title">高级请求参数</span><small>Headers、Params 与认证</small></template>
                <div class="form-grid advanced-grid">
                  <el-form-item class="full-row" label="请求 Headers"><key-value-editor v-model="form.headers" key-placeholder="Header 名称" value-placeholder="Header 值" /></el-form-item>
                  <el-form-item class="full-row" label="查询 Params"><key-value-editor v-model="form.params" key-placeholder="参数名" value-placeholder="参数值" /></el-form-item>
                  <el-form-item label="Content-Type"><el-input v-model="form.content_type" /></el-form-item>
                  <el-form-item label="认证方式"><el-select v-model="form.auth_type"><el-option label="无需认证" value="none" /><el-option label="Basic Auth" value="basic" /><el-option label="Digest Auth" value="digest" /></el-select></el-form-item>
                  <template v-if="form.auth_type !== 'none'"><el-form-item label="用户名" prop="username"><el-input v-model="form.username" /></el-form-item><el-form-item label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item></template>
                </div>
              </el-collapse-item>
            </el-collapse>
          </div>

          <div v-else class="form-grid">
            <el-form-item class="full-row" label="目标 Host" prop="host"><el-input v-model="form.host" placeholder="IP 地址或域名" /></el-form-item>
            <el-form-item v-if="form.type !== 'icmp'" label="端口"><el-input-number v-model="form.port" :min="1" :max="65535" /></el-form-item>
            <el-form-item v-if="form.type === 'icmp'" label="发包数量"><el-input-number v-model="form.count" :min="1" :max="10" /></el-form-item>
            <el-form-item label="超时时间"><el-input-number v-model="form.timeout_ms" :min="1000" :max="60000" :step="1000" /><span class="field-unit">毫秒</span></el-form-item>
            <el-form-item v-if="form.type === 'certificate_expiry'" class="full-row" label="SNI / Server Name"><el-input v-model="form.server_name" placeholder="默认使用目标域名，可按需覆盖" /></el-form-item>
          </div>

        </section>

        <section class="form-section">
          <div class="section-title"><strong>执行节点</strong><span>选择发起本次拨测的位置</span></div>
          <div class="execution-panel">
            <p>默认由服务端直接发起；探测内网或指定网络区域时，可切换到在线的 SSH 或 Agent 节点。</p>
            <el-select v-model="form.node_value" aria-label="执行节点">
              <el-option label="服务端直连" value="direct" />
              <el-option-group v-if="sshNodes.length" label="SSH 节点"><el-option v-for="node in sshNodes" :key="`ssh:${node.id}`" :label="node.name" :value="`ssh:${node.id}`" :disabled="node.status !== 1" /></el-option-group>
              <el-option-group v-if="hostAgentNodes.length" label="Agent 节点"><el-option v-for="node in hostAgentNodes" :key="`agent:${node.id}`" :label="node.name" :value="`agent:${node.id}`" :disabled="node.status !== 'online'" /></el-option-group>
              <el-option-group v-if="dockerAgentNodes.length" label="Docker 节点"><el-option v-for="node in dockerAgentNodes" :key="`agent:${node.id}`" :label="node.name" :value="`agent:${node.id}`" :disabled="node.status !== 'online'" /></el-option-group>
            </el-select>
          </div>
        </section>

        <section class="form-section schedule-section">
          <div class="section-title"><strong>调度与告警</strong><span>控制执行频率及何时发送失败通知</span></div>
          <div class="schedule-grid">
            <el-form-item label="多久拨测一次"><el-select v-model="form.interval_seconds"><el-option v-for="item in intervals" :key="item.value" :label="item.label" :value="item.value" /></el-select></el-form-item>
            <el-form-item label="连续失败"><div class="threshold-field"><el-input-number v-model="form.failure_threshold" :min="1" :max="100" /><span>次后通知</span></div></el-form-item>
            <el-form-item label="故障中心" prop="fault_center_id"><el-select v-model="form.fault_center_id" filterable placeholder="请选择故障中心"><el-option v-for="center in activeCenters" :key="center.id" :label="center.name" :value="center.id" /></el-select></el-form-item>
            <el-form-item label="任务状态"><div class="status-field"><el-switch v-model="form.status" :active-value="1" :inactive-value="0" /><span>{{ form.status === 1 ? '启用后自动执行' : '暂不自动执行' }}</span></div></el-form-item>
          </div>
          <div class="alert-note"><strong>告警降噪</strong><span>达到连续失败阈值时只通知一次；任务恢复成功后清零，下一轮连续失败才会再次通知。</span></div>
        </section>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button @click="drawerVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="handleSubmit">{{ isEdit ? '保存修改' : '创建任务' }}</el-button></div></template>
    </el-drawer>

  </div>
</template>

<script setup>
import { usePageBreadcrumb } from '../../composables/usePageBreadcrumb'
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, defineComponent, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElButton, ElInput, ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, CopyDocument, Delete, Download, Edit, Plus, Refresh, Search, Upload } from '@element-plus/icons-vue'
import { getInspectionRuleGroup, getInspectionItems, createInspectionItem, updateInspectionItem, deleteInspectionItem, batchDeleteInspectionItems, batchUpdateInspectionItemStatus, exportInspectionItems, importInspectionItems } from '../../api/inspection'
import { probeTypes } from '../../utils/probeTypes'
import { getAgents, getCertificateExecutors } from '../../api/certificate'
import { getFaultCenters } from '../../api/alert'
import { isDockerAgent } from '../../utils/agentNode'

const KeyValueEditor = defineComponent({
  name: 'KeyValueEditor', props: { modelValue: { type: Array, required: true }, keyPlaceholder: { type: String, default: '键' }, valuePlaceholder: { type: String, default: '值' } }, emits: ['update:modelValue'],
  setup(props, { emit }) {
    const update = (index, field, value) => emit('update:modelValue', props.modelValue.map((row, i) => i === index ? { ...row, [field]: value } : row))
    const add = () => emit('update:modelValue', [...props.modelValue, { key: '', value: '' }])
    const remove = index => { const rows = props.modelValue.filter((_, i) => i !== index); emit('update:modelValue', rows.length ? rows : [{ key: '', value: '' }]) }
    return () => h('div', { class: 'kv-editor' }, props.modelValue.map((row, index) => h('div', { class: 'kv-row', key: index }, [h(ElInput, { modelValue: row.key, placeholder: props.keyPlaceholder, 'onUpdate:modelValue': value => update(index, 'key', value) }), h(ElInput, { modelValue: row.value, placeholder: props.valuePlaceholder, 'onUpdate:modelValue': value => update(index, 'value', value) }), h(ElButton, { icon: index === props.modelValue.length - 1 ? Plus : Delete, circle: true, onClick: index === props.modelValue.length - 1 ? add : () => remove(index), 'aria-label': index === props.modelValue.length - 1 ? '新增一行' : '删除此行' })])))
  },
})

const taskTypes = probeTypes
const intervals = [{ label: '每 30 秒', value: 30 }, { label: '每 1 分钟', value: 60 }, { label: '每 5 分钟', value: 300 }, { label: '每 10 分钟', value: 600 }, { label: '每 30 分钟', value: 1800 }, { label: '每 1 小时', value: 3600 }, { label: '每 6 小时', value: 21600 }, { label: '每 24 小时', value: 86400 }]
const httpMethods = ['GET', 'POST', 'PUT', 'DELETE']
const defaultForm = (type = 'http') => ({ id: null, name: '', type, target: '', host: '', port: taskTypes.find(item => item.value === type)?.defaultPort || 80, method: 'GET', headers: [{ key: '', value: '' }], params: [{ key: '', value: '' }], body: '', timeout_ms: 10000, content_type: 'application/json', auth_type: 'none', username: '', password: '', success_codes: '200-399', count: 4, server_name: '', node_value: 'direct', interval_seconds: 300, failure_threshold: 3, fault_center_id: null, status: 1 })

const loading = ref(false), saving = ref(false), drawerVisible = ref(false), isEdit = ref(false), formRef = ref()
const router = useRouter(), route = useRoute(), group = ref(null)
usePageBreadcrumb(() => group.value?.name)
const groupId = computed(() => Number(route.params.groupId))
const tableData = ref([]), selectedRows = ref([]), batchAction = ref(''), togglingId = ref(null), total = ref(0), page = ref(1), pageSize = ref(20), sshNodes = ref([]), agentNodes = ref([]), faultCenters = ref([]), form = reactive(defaultForm())
const hostAgentNodes = computed(() => agentNodes.value.filter(item => !isDockerAgent(item)))
const dockerAgentNodes = computed(() => agentNodes.value.filter(isDockerAgent))
const filters = reactive({ keyword: '', type: '', status: '' })
const importVisible = ref(false), importContent = ref(''), importOverwrite = ref(false), importPreview = ref(null), validatingImport = ref(false), importing = ref(false)
const importExample = `apiVersion: aiops/v1
kind: InspectionTaskList
tasks:
  - name: 示例 HTTP 拨测
    type: http
    target: https://example.com/health
    config:
      execution_nodes:
        - node_type: direct
          node_id: 0
      method: GET
      target: https://example.com/health
      timeout_ms: 10000
      success_codes: 200-399
    external_labels:
      env: production
    interval_seconds: 60
    failure_threshold: 3
    recovery_threshold: 3
    severity: P2
    fault_center_id: 1
    status: 1`
let keywordSearchTimer
const currentType = computed(() => typeMeta(form.type)), requestBodyVisible = computed(() => ['POST', 'PUT'].includes(form.method))
const activeCenters = computed(() => faultCenters.value.filter(item => item.status === 1))
const rules = {
  name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }],
  target: [{ validator: (_r, value, done) => form.type !== 'http' || /^https?:\/\/[^\s]+$/i.test(String(value || '').trim()) ? done() : done(new Error('请输入以 http:// 或 https:// 开头的完整 URL')), trigger: 'blur' }],
  host: [{ validator: (_r, value, done) => !['icmp', 'tcp', 'certificate_expiry'].includes(form.type) || String(value || '').trim() ? done() : done(new Error('请输入目标 Host')), trigger: 'blur' }],
  fault_center_id: [{ required: true, message: '请选择故障中心', trigger: 'change' }],
  username: [{ validator: (_r, value, done) => form.auth_type === 'none' || String(value || '').trim() ? done() : done(new Error('请输入认证用户名')), trigger: 'blur' }],
}

onMounted(fetchData)
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))
async function fetchData() {
  loading.value = true
  try {
    const results = await Promise.allSettled([getInspectionItems(taskQuery()), getCertificateExecutors(), getAgents(), getFaultCenters(), getInspectionRuleGroup(groupId.value)])
    if (results[0].status === 'fulfilled') applyTaskPage(results[0].value.data)
    if (results[1].status === 'fulfilled') sshNodes.value = results[1].value.data || []
    if (results[2].status === 'fulfilled') agentNodes.value = results[2].value.data || []
    if (results[3].status === 'fulfilled') faultCenters.value = results[3].value.data || []
    if (results[4].status === 'fulfilled') group.value = results[4].value.data
    else router.replace('/inspection/items')
  } finally { loading.value = false }
}
function cleanFilters() { return Object.fromEntries(Object.entries(filters).map(([key, value]) => [key, String(value || '').trim()]).filter(([, value]) => value)) }
function taskQuery() { return { group_id: groupId.value, ...cleanFilters(), page: page.value, page_size: pageSize.value } }
function applyTaskPage(data) { tableData.value = data?.list || []; selectedRows.value = []; total.value = Number(data?.total || 0) }
async function loadTaskPage() { loading.value = true; try { const res = await getInspectionItems(taskQuery()); applyTaskPage(res.data) } finally { loading.value = false } }
async function handleSearch() { clearTimeout(keywordSearchTimer); page.value = 1; await loadTaskPage() }
function handleKeywordInput() { clearTimeout(keywordSearchTimer); keywordSearchTimer = setTimeout(handleSearch, 300) }
function clearFilters() { Object.assign(filters, { keyword: '', type: '', status: '' }); handleSearch() }
async function handlePageSizeChange() { page.value = 1; await loadTaskPage() }
function handleSelectionChange(rows) { selectedRows.value = rows }
function handleBatchCommand(command) {
  if (command === 'enable') return handleBatchStatus(1)
  if (command === 'disable') return handleBatchStatus(0)
  if (command === 'delete') return handleBatchDelete()
  if (command === 'import') return openImportDialog()
  if (command === 'export') return downloadInspectionConfig()
}
function openImportDialog() {
  resetImportDialog()
  importVisible.value = true
}
function resetImportDialog() {
  importContent.value = ''
  importOverwrite.value = false
  importPreview.value = null
}
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
    const response = await importInspectionItems(importContent.value, importOverwrite.value, true, groupId.value)
    importPreview.value = response.data
    ElMessage.success('配置校验通过')
  } finally { validatingImport.value = false }
}
async function confirmImportConfig() {
  if (!importPreview.value) return
  const summary = importPreview.value
  const confirmed = await ElMessageBox.confirm(
    `确认导入 ${summary.total} 条配置吗？将新增 ${summary.created} 条、覆盖 ${summary.updated} 条、跳过 ${summary.skipped} 条。`,
    '确认导入',
    { type: summary.updated ? 'warning' : 'info', confirmButtonText: '导入', cancelButtonText: '取消' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return
  importing.value = true
  try {
    const response = await importInspectionItems(importContent.value, importOverwrite.value, false, groupId.value)
    const result = response.data
    ElMessage.success(`导入完成：新增 ${result.created} 条，覆盖 ${result.updated} 条，跳过 ${result.skipped} 条`)
    importVisible.value = false
    page.value = 1
    await fetchData()
  } finally { importing.value = false }
}
async function downloadInspectionConfig() {
  const response = await exportInspectionItems({ group_id: groupId.value })
  const blob = response.data instanceof Blob ? response.data : new Blob([response.data])
  const url = window.URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = downloadFilename(response.headers?.['content-disposition']) || 'inspection-tasks.yaml'
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
    await batchUpdateInspectionItemStatus(selectedRows.value.map(row => row.id), status)
    ElMessage.success(`已批量${status === 1 ? '启用' : '禁用'} ${count} 个任务`)
    await loadTaskPage()
  } finally { batchAction.value = '' }
}
async function handleBatchDelete() {
  if (!selectedRows.value.length) return
  const count = selectedRows.value.length
  const confirmed = await ElMessageBox.confirm(
    `确定删除选中的 ${count} 个拨测任务吗？同时会删除这些任务的拨测记录，删除后无法恢复。`,
    '批量删除',
    { type: 'warning', confirmButtonText: '删除', cancelButtonText: '取消' }
  ).then(() => true).catch(() => false)
  if (!confirmed) return

  batchAction.value = 'delete'
  try {
    await batchDeleteInspectionItems(selectedRows.value.map(row => row.id))
    ElMessage.success(`已删除 ${count} 个拨测任务`)
    await loadTaskPage()
  } finally { batchAction.value = '' }
}
async function toggleTaskStatus(row) {
  const status = row.status === 1 ? 0 : 1
  togglingId.value = row.id
  try {
    await batchUpdateInspectionItemStatus([row.id], status)
    ElMessage.success(`任务已${status === 1 ? '启用' : '停用'}`)
    return true
  } catch {
    return false
  } finally { togglingId.value = null }
}
function typeMeta(type) { return taskTypes.find(item => item.value === type) || { label: String(type || '-').toUpperCase(), title: '未知类型', hint: '' } }
function changeType(type) { if (form.type === type) return; Object.assign(form, { ...defaultForm(type), name: form.name, interval_seconds: form.interval_seconds, failure_threshold: form.failure_threshold, fault_center_id: form.fault_center_id, status: form.status }); formRef.value?.clearValidate?.() }
function handleAdd() { router.push({ path: '/inspection/items/create', query: { group_id: groupId.value } }) }
function handleCopy(row) { router.push({ path: '/inspection/items/create', query: { group_id: row.group_id || groupId.value, copy_from: row.id } }) }
function handleDetail(row) { router.push(`/inspection/items/${row.id}`) }
function handleEdit(row) {
  router.push(`/inspection/items/${row.id}/edit`)
}
function buildPayload() {
  const [nodeType, rawNodeID] = form.node_value.includes(':') ? form.node_value.split(':') : ['direct', '0']
  const common = { node_type: nodeType, node_id: Number(rawNodeID) || 0, timeout_ms: form.timeout_ms }
  let config
  if (form.type === 'http') config = { ...common, target: form.target.trim(), method: form.method, headers: rowsToObject(form.headers), params: rowsToObject(form.params), body: form.body, content_type: form.content_type, auth_type: form.auth_type, username: form.username, password: form.password, success_codes: form.success_codes }
  else config = { ...common, host: form.host.trim(), port: form.port, count: form.count, server_name: form.server_name.trim() }
  const target = form.type === 'http' ? form.target.trim() : form.host.trim()
  return { name: form.name.trim(), type: form.type, target, config: JSON.stringify(config), expected_result: '拨测成功', interval_seconds: form.interval_seconds, failure_threshold: form.failure_threshold, fault_center_id: form.fault_center_id || 0, status: form.status }
}
async function handleSubmit() { if (!await formRef.value.validate().catch(() => false)) return; saving.value = true; try { const payload = buildPayload(); form.id ? await updateInspectionItem(form.id, payload) : await createInspectionItem(payload); ElMessage.success(form.id ? '任务已更新' : '任务已创建'); drawerVisible.value = false; await fetchData() } finally { saving.value = false } }
async function handleDelete(row) { await ElMessageBox.confirm(`确定删除拨测任务「${row.name}」吗？\n同时会删除该任务的拨测记录。`, '提示', { type: 'warning' }); await deleteInspectionItem(row.id); ElMessage.success('已删除'); fetchData() }
function rowsToObject(rows) { return rows.reduce((result, row) => { const key = String(row.key || '').trim(); if (key) result[key] = row.value || ''; return result }, {}) }
function objectToRows(value) { const rows = Object.entries(value || {}).map(([key, entry]) => ({ key, value: entry })); return rows.length ? rows : [{ key: '', value: '' }] }
function faultCenterName(id) { return id ? faultCenters.value.find(item => item.id === id)?.name || '故障中心已删除' : '未分配故障中心' }
function severityTagType(severity) { return severity === 'P0' ? 'danger' : severity === 'P1' ? 'warning' : 'info' }
function executionNodes(row) {
  if (Array.isArray(row.execution_nodes) && row.execution_nodes.length) return row.execution_nodes
  let config = {}; try { config = JSON.parse(row.config || '{}') } catch {}
  const nodes = Array.isArray(config.execution_nodes) ? config.execution_nodes : []
  return nodes.map(node => ({ ...node, node_name: node.node_name || ({ direct: '服务端直连', ssh: `SSH #${node.node_id}`, agent: `Agent #${node.node_id}` })[node.node_type || 'direct'], state: 'not_run' }))
}
function nodeStateSummary(row) {
  const nodes = executionNodes(row)
  const counts = nodes.reduce((result, node) => { result[node.state || 'not_run'] = (result[node.state || 'not_run'] || 0) + 1; return result }, {})
  const total = nodes.length, normal = (counts.success || 0) + (counts.recovering || 0), failed = counts.failed || 0, notRun = counts.not_run || 0
  if (normal === total) return [{ key: 'normal', label: '全部正常', type: 'success' }]
  if (failed === total) return [{ key: 'failed', label: '全部异常', type: 'danger' }]
  if (notRun === total) return [{ key: 'not_run', label: '全部待测', type: 'info' }]
  return [
    normal ? { key: 'normal', label: `${normal} 个正常`, type: 'success' } : null,
    failed ? { key: 'failed', label: `${failed} 个异常`, type: 'danger' } : null,
    notRun ? { key: 'not_run', label: `${notRun} 个待测`, type: 'info' } : null,
  ].filter(Boolean)
}
function formatDuration(seconds) { if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600} 小时`; if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60} 分钟`; return `${seconds} 秒` }
function formatShortTime(value) { const date = new Date(value); return Number.isNaN(date.getTime()) ? '' : date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }) }
</script>

<style scoped>

.task-page { --ink: var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); display: flex; flex-direction: column; gap: 16px; }
.task-hero { display: flex; align-items: center; gap: 16px; min-height: 92px; padding: 20px 24px; background: linear-gradient(115deg, #fff 0%, #f6f9fe 60%, #edf5ff 100%); border: 1px solid #dce7f6; border-radius: 10px; box-shadow: 0 8px 28px rgba(35, 78, 134, .06); }
.hero-icon { display: inline-flex; align-items: center; justify-content: center; width: 48px; height: 48px; flex: 0 0 48px; color: #fff; background: linear-gradient(145deg, #2478e8, #0f5bc6); border-radius: 12px; box-shadow: 0 8px 18px rgba(23, 105, 224, .22); font-size: 24px; }
.hero-copy { margin-right: auto; }.hero-copy h1 { margin: 0; color: var(--ink); font-size: 22px; font-weight: 750; }.hero-copy p { margin: 6px 0 0; color: var(--muted); font-size: 13px; }.hero-actions { display: flex; gap: 10px; }
.task-card { border-color: var(--border); border-radius: 8px; box-shadow: none; }.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }
.filter-bar { display: flex; min-width: 0; flex: 1; align-items: center; gap: 8px; }
.task-search { width: 180px; }
.filter-select { flex: 0 0 auto; }
.filter-type { width: 110px; }
.filter-status { width: 110px; }
.task-name,.last-status { display: flex; min-width: 0; flex-direction: column; gap: 3px; }.task-name-link { align-self:flex-start;max-width:100%;padding:0;overflow:hidden;color:var(--ops-primary);font:inherit;font-size:13px;font-weight:700;text-align:left;text-overflow:ellipsis;white-space:nowrap;cursor:pointer;background:none;border:0 }.task-name-link:hover{text-decoration:underline}.task-name-link:focus-visible{outline:2px solid rgba(23,105,224,.3);outline-offset:2px;border-radius:2px}.task-name small,.last-status small { overflow: hidden; color: #8a96a8; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }.last-status { align-items: flex-start; }.node-tags{display:flex;flex-wrap:wrap;gap:5px}.execution-node-tag{max-width:100%;border-radius:2px}.execution-node-tag :deep(.el-tag__content){overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.task-type-text { color: #4d5d73; font-size: 12px; }
.severity-tag{min-width:36px;justify-content:center;border-radius:0;font-weight:600}
.node-state-list{display:flex;align-items:center;flex-direction:column;gap:4px}.node-state-tag{border-radius:0}
.task-form { padding: 0 2px 24px; }.form-section { margin-bottom: 18px; padding: 20px; background: var(--ops-surface); border: 1px solid var(--border); border-radius: 10px; }.section-title { display: flex; align-items: baseline; gap: 10px; margin-bottom: 18px; }.section-title strong { color: var(--ink); font-size: 15px; }.section-title span { color: #8a96a8; font-size: 12px; }
.probe-types { display: grid; grid-template-columns: repeat(5,minmax(0,1fr)); gap: 9px; }.type-card { display: flex; min-width: 0; flex-direction: column; gap: 3px; padding: 12px; color: #617087; text-align: left; cursor: pointer; background: #fafbfd; border: 1px solid var(--ops-border); border-radius: 8px; transition: .18s ease; }.type-card:hover { border-color: #a9c8ee; transform: translateY(-1px); }.type-card:focus-visible { outline: 3px solid rgba(23,105,224,.16); outline-offset: 2px; }.type-card.active { color: #1c5fb9; background: #f3f8ff; border-color: #6da5ea; box-shadow: 0 5px 14px rgba(23,105,224,.08); }.type-card>span { font-size: 10px; font-weight: 800; letter-spacing: .4px; }.type-card strong { color: #28384f; font-size: 13px; white-space: nowrap; }.type-card small { overflow: hidden; color: var(--ops-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.form-grid,.schedule-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 0 20px; }.full-row { grid-column: 1/-1; }:deep(.el-form-item) { margin-bottom: 18px; }:deep(.el-form-item__label) { padding-bottom: 7px; color: #46536a; font-size: 13px; font-weight: 650; }:deep(.el-select),:deep(.el-input-number) { width: 100%; }
.method-field :deep(.el-radio-group) { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); width: 100%; }.method-field :deep(.el-radio-button),.method-field :deep(.el-radio-button__inner) { width: 100%; }.field-unit { position: absolute; right: 12px; color: #8a96a8; font-size: 12px; pointer-events: none; }.mono-input :deep(textarea),.run-result pre,.record-result { font-family: 'SF Mono',Monaco,Menlo,Consolas,monospace; }
.advanced-collapse { margin-top: 2px; overflow: hidden; background: #f8fafc; border: 1px solid #e1e8f1; border-radius: 8px; }.advanced-collapse :deep(.el-collapse-item__header) { height: 50px; padding: 0 15px; background: #f8fafc; border: 0; }.advanced-collapse :deep(.el-collapse-item__wrap) { border: 0; }.advanced-collapse :deep(.el-collapse-item__content) { padding: 16px 16px 0; }.advanced-title { color: #315781; font-weight: 700; }.advanced-title+small { margin-left: 8px; color: #8a96a8; }
.execution-panel { display: grid; grid-template-columns: minmax(0,1fr) minmax(280px,360px); gap: 28px; align-items: center; padding: 16px 18px; background: linear-gradient(105deg,#f8fafd,#fbfcfe); border: 1px solid #dfe7f1; border-radius: 8px; }.execution-panel p { margin: 0; color: #7d8b9f; font-size: 12px; line-height: 1.6; }
.schedule-section { background: #fbfcfe; }.schedule-grid { grid-template-columns: repeat(4,minmax(0,1fr)); }.threshold-field,.status-field { display: flex; width: 100%; align-items: center; gap: 8px; color: #69778a; font-size: 12px; }.threshold-field :deep(.el-input-number) { min-width: 0; flex: 1; }.alert-note { display: flex; gap: 8px; padding: 11px 13px; color: #755922; background: #fffaf0; border: 1px solid #f1e3bf; border-radius: 7px; font-size: 12px; line-height: 1.55; }.alert-note strong { white-space: nowrap; }.drawer-footer { display: flex; justify-content: flex-end; gap: 10px; }
.run-result :deep(.el-result) { padding: 12px 0; }.run-result pre { max-height: 280px; margin: 0; padding: 14px; overflow: auto; color: #d7e0ec; background: #111b2e; border-radius: 8px; font-size: 12px; line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; }.record-result { display: block; max-height: 100px; overflow: auto; font-size: 11px; line-height: 1.5; white-space: pre-wrap; overflow-wrap: anywhere; }.pagination { display: flex; justify-content: flex-end; margin-top: 14px; }
:deep(.kv-editor) { display: flex; width: 100%; flex-direction: column; gap: 8px; }:deep(.kv-row) { display: grid; grid-template-columns: 1fr 1fr 34px; gap: 9px; }:deep(.kv-row .el-button) { width: 32px; height: 32px; }
@media (prefers-reduced-motion:reduce) { .type-card { transition: none; } } @media (max-width:900px) { .probe-types { grid-template-columns: repeat(3,minmax(0,1fr)); }.schedule-grid { grid-template-columns: repeat(2,minmax(0,1fr)); } } @media (max-width:700px) { .task-hero { align-items: flex-start; flex-wrap: wrap; padding: 18px; }.hero-copy { width: calc(100% - 64px); }.hero-actions { width: 100%; justify-content: flex-end; }.probe-types,.form-grid,.schedule-grid { grid-template-columns: 1fr; }.execution-panel { grid-template-columns: 1fr; gap: 12px; }.full-row { grid-column: auto; } }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.task-toolbar .filter-bar {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: 220px 110px 110px auto;
  gap: 8px;
}
.task-toolbar .task-search,
.task-toolbar .filter-select { width: 100%; }
.task-toolbar .page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.task-toolbar .page-header-actions :deep(.el-button + .el-button) { margin-left: 0; }
.batch-operation-arrow { margin-left: 6px; }
.import-note { display: flex; align-items: flex-start; gap: 10px; margin-bottom: 14px; padding: 11px 13px; color: #637083; background: #f3f8ff; border-left: 3px solid var(--ops-primary); font-size: 12px; line-height: 1.6; }
.import-note strong { flex: 0 0 auto; color: #315781; }
.import-actions { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; color: #8a96a8; font-size: 12px; }
.import-editor :deep(textarea) { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; line-height: 1.55; }
.inspection-import-dialog { display: flex; max-height: 90vh; flex-direction: column; }
.inspection-import-dialog :deep(.el-dialog__body) { min-height: 0; overflow-y: auto; }
.import-policy { display: flex; align-items: center; justify-content: space-between; gap: 18px; margin-top: 14px; padding: 12px 14px; border: 1px solid #e1e8f1; border-radius: 7px; background: #fafbfd; }
.import-policy > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; }.import-policy strong { color: #344054; font-size: 13px; }.import-policy span { color: #8a96a8; font-size: 11px; line-height: 1.5; }
.import-preview { display: flex; align-items: center; flex-wrap: wrap; gap: 14px; margin-top: 12px; padding: 10px 12px; color: var(--ops-muted); background: #f7f9fc; font-size: 12px; }.import-preview strong { color: var(--ops-muted); }.import-preview .created { color: #2f855a; }.import-preview .updated { color: #b7791f; }
@media (max-width: 1320px) {
  .task-toolbar { align-items: stretch; flex-wrap: wrap; }
  .task-toolbar .filter-bar { width: 100%; flex: 1 1 100%; }
  .task-toolbar .page-header-actions { margin-left: auto; }
}
@media (max-width: 700px) {
  .task-toolbar { align-items: stretch; flex-direction: column; }
  .task-toolbar .filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .task-toolbar .task-search { grid-column: 1 / -1; }
  .task-toolbar .page-header-actions { justify-content: flex-end; }
}

.probe-task-drawer .probe-types { grid-template-columns:repeat(3,minmax(0,1fr)); }
.probe-task-drawer .form-grid,.probe-task-drawer .schedule-grid,.probe-task-drawer .execution-panel { grid-template-columns:minmax(0,1fr); gap:12px; }
.probe-task-drawer .full-row { grid-column:auto; }
.probe-task-drawer .section-title { flex-wrap:wrap; }
</style>

<style>
.probe-task-drawer .task-form { padding: 0 2px 24px; }
.probe-task-drawer .form-section { margin-bottom: 18px; padding: 20px; background: var(--ops-surface); border: 1px solid #e1e8f1; border-radius: 10px; }
.probe-task-drawer .section-title { display: flex; align-items: baseline; gap: 10px; margin-bottom: 18px; }
.probe-task-drawer .section-title strong { color: var(--ops-text); font-size: 15px; }
.probe-task-drawer .section-title span { color: #8a96a8; font-size: 12px; }
.probe-task-drawer .probe-types { display: grid; grid-template-columns: repeat(5,minmax(0,1fr)); gap: 9px; }
.probe-task-drawer .type-card { display: flex; min-width: 0; flex-direction: column; gap: 3px; padding: 12px; color: #617087; text-align: left; cursor: pointer; background: #fafbfd; border: 1px solid var(--ops-border); border-radius: 8px; transition: border-color .18s ease, box-shadow .18s ease, transform .18s ease; }
.probe-task-drawer .type-card:hover { border-color: #a9c8ee; transform: translateY(-1px); }
.probe-task-drawer .type-card:focus-visible { outline: 3px solid rgba(23,105,224,.16); outline-offset: 2px; }
.probe-task-drawer .type-card.active { color: #1c5fb9; background: #f3f8ff; border-color: #6da5ea; box-shadow: 0 5px 14px rgba(23,105,224,.08); }
.probe-task-drawer .type-card > span { font-size: 10px; font-weight: 800; letter-spacing: .4px; }
.probe-task-drawer .type-card strong { color: #28384f; font-size: 13px; white-space: nowrap; }
.probe-task-drawer .type-card small { overflow: hidden; color: var(--ops-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.probe-task-drawer .form-grid,
.probe-task-drawer .schedule-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 0 20px; }
.probe-task-drawer .full-row { grid-column: 1/-1; }
.probe-task-drawer .el-form-item { margin-bottom: 18px; }
.probe-task-drawer .el-form-item__label { padding-bottom: 7px; color: #46536a; font-size: 13px; font-weight: 650; }
.probe-task-drawer .el-select,
.probe-task-drawer .el-input-number { width: 100%; }
.probe-task-drawer .method-field .el-radio-group { display: grid; grid-template-columns: repeat(4,minmax(0,1fr)); width: 100%; }
.probe-task-drawer .method-field .el-radio-button,
.probe-task-drawer .method-field .el-radio-button__inner { width: 100%; }
.probe-task-drawer .field-unit { position: absolute; right: 12px; color: #8a96a8; font-size: 12px; pointer-events: none; }
.probe-task-drawer .mono-input textarea { font-family: 'SF Mono',Monaco,Menlo,Consolas,monospace; }
.probe-task-drawer .advanced-collapse { margin-top: 2px; overflow: hidden; background: #f8fafc; border: 1px solid #e1e8f1; border-radius: 8px; }
.probe-task-drawer .advanced-collapse .el-collapse-item__header { height: 50px; padding: 0 15px; background: #f8fafc; border: 0; }
.probe-task-drawer .advanced-collapse .el-collapse-item__wrap { border: 0; }
.probe-task-drawer .advanced-collapse .el-collapse-item__content { padding: 16px 16px 0; }
.probe-task-drawer .advanced-title { color: #315781; font-weight: 700; }
.probe-task-drawer .advanced-title + small { margin-left: 8px; color: #8a96a8; }
.probe-task-drawer .execution-panel { display: grid; grid-template-columns: minmax(0,1fr) minmax(280px,360px); gap: 28px; align-items: center; padding: 16px 18px; background: linear-gradient(105deg,#f8fafd,#fbfcfe); border: 1px solid #dfe7f1; border-radius: 8px; }
.probe-task-drawer .execution-panel p { margin: 0; color: #7d8b9f; font-size: 12px; line-height: 1.6; }
.probe-task-drawer .schedule-section { background: #fbfcfe; }
.probe-task-drawer .schedule-grid { grid-template-columns: repeat(4,minmax(0,1fr)); }
.probe-task-drawer .threshold-field,
.probe-task-drawer .status-field { display: flex; width: 100%; align-items: center; gap: 8px; color: #69778a; font-size: 12px; }
.probe-task-drawer .threshold-field .el-input-number { min-width: 0; flex: 1; }
.probe-task-drawer .alert-note { display: flex; gap: 8px; padding: 11px 13px; color: #755922; background: #fffaf0; border: 1px solid #f1e3bf; border-radius: 7px; font-size: 12px; line-height: 1.55; }
.probe-task-drawer .alert-note strong { white-space: nowrap; }
.probe-task-drawer .drawer-footer { display: flex; justify-content: flex-end; gap: 10px; }
.probe-task-drawer .kv-editor { display: flex; width: 100%; flex-direction: column; gap: 8px; }
.probe-task-drawer .kv-row { display: grid; grid-template-columns: 1fr 1fr 34px; gap: 9px; }
.probe-task-drawer .kv-row .el-button { width: 32px; height: 32px; }
@media (prefers-reduced-motion:reduce) { .probe-task-drawer .type-card { transition: none; } }
@media (max-width:900px) { .probe-task-drawer .probe-types { grid-template-columns: repeat(3,minmax(0,1fr)); } .probe-task-drawer .schedule-grid { grid-template-columns: repeat(2,minmax(0,1fr)); } }
@media (max-width:700px) { .probe-task-drawer .probe-types,.probe-task-drawer .form-grid,.probe-task-drawer .schedule-grid { grid-template-columns: 1fr; } .probe-task-drawer .execution-panel { grid-template-columns: 1fr; gap: 12px; } .probe-task-drawer .full-row { grid-column: auto; } }

.probe-task-drawer .probe-types { grid-template-columns:repeat(3,minmax(0,1fr)); }
.probe-task-drawer .form-grid,.probe-task-drawer .schedule-grid,.probe-task-drawer .execution-panel { grid-template-columns:minmax(0,1fr); gap:12px; }
.probe-task-drawer .full-row { grid-column:auto; }
.probe-task-drawer .section-title { flex-wrap:wrap; }
</style>

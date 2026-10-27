<template>
  <div class="page-container">
    <div class="datasource-toolbar">
      <div class="datasource-filters" role="search" aria-label="监控实例筛选">
        <el-input v-model="keyword" class="filter-keyword" :prefix-icon="Search" clearable aria-label="搜索数据源实例" placeholder="搜索名称、地址或标签" @input="handleKeywordInput" @keyup.enter="handleSearch" @clear="handleSearch" />
        <el-select v-model="typeFilter" class="filter-type" aria-label="按类型筛选" placeholder="全部类型" clearable @change="handleSearch"><el-option v-for="item in datasourceTypes" :key="item.type" :label="item.name" :value="item.type" /></el-select>
        <el-select v-model="statusFilter" class="filter-status" aria-label="按状态筛选" placeholder="全部状态" clearable @change="handleSearch"><el-option label="启用" value="1" /><el-option label="禁用" value="0" /></el-select>
        <el-button @click="handleReset">清空</el-button>
      </div>
      <div class="datasource-actions">
        <el-dropdown trigger="click" :disabled="!selectedRows.length || !!batchAction" @command="handleBatchCommand">
          <el-button :disabled="!selectedRows.length || !!batchAction" :loading="!!batchAction">批量操作<el-icon class="batch-arrow"><ArrowDown /></el-icon></el-button>
          <template #dropdown><el-dropdown-menu><el-dropdown-item command="enable" v-permission="['datasource:edit']">批量启用</el-dropdown-item><el-dropdown-item command="disable" v-permission="['datasource:edit']">批量禁用</el-dropdown-item><el-dropdown-item command="delete" divided v-permission="['datasource:delete']">批量删除</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
        <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
        <el-button type="primary" :icon="Plus" :disabled="requireGroup && !groupId" @click="handleAdd" v-permission="['datasource:create']">新增实例</el-button>
      </div>
    </div>
    <el-card class="datasource-table-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" row-key="id" border @selection-change="selectedRows = $event" @sort-change="handleSortChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="name" label="实例名称" min-width="180" sortable="custom"><template #default="{ row }"><span class="instance-name"><img v-if="row.type === 'prometheus'" :src="prometheusLogo" alt="" /><img v-else-if="row.type === 'victoriametrics'" :src="victoriaMetricsLogo" alt="" class="vm-logo" /><img v-else-if="row.type === 'loki'" :src="lokiLogo" alt="" class="loki-logo" /><img v-else-if="row.type === 'elasticsearch'" :src="elasticsearchLogo" alt="" class="log-source-logo" /><img v-else-if="row.type === 'clickhouse'" :src="clickhouseLogo" alt="" class="log-source-logo" /><img v-else-if="row.type === 'jaeger'" :src="jaegerLogo" alt="" class="jaeger-logo" /><el-icon v-else><Connection /></el-icon>{{ row.name }}</span></template></el-table-column>
        <el-table-column prop="type" label="类型" min-width="160"><template #default="{ row }">{{ typeDefinition(row.type).name || row.type }}</template></el-table-column>
        <el-table-column prop="url" label="接入地址" min-width="230" show-overflow-tooltip />
        <el-table-column label="标签" min-width="180"><template #default="{ row }"><span class="scope-tags"><el-tag v-for="(value, key) in parseObject(row.labels)" :key="key" size="small" effect="plain">{{ key }}:{{ value }}</el-tag></span></template></el-table-column>
        <el-table-column prop="remark" label="备注" min-width="150" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100" sortable="custom"><template #default="{ row }"><span class="status"><i :class="{ enabled: row.status === 1 }" />{{ row.status === 1 ? '启用' : '禁用' }}</span></template></el-table-column>
        <el-table-column label="操作" width="110" fixed="right"><template #default="{ row }"><el-button link type="primary" :icon="Edit" aria-label="编辑实例" title="编辑" @click="handleEdit(row)" v-permission="['datasource:edit']" /><el-button link type="danger" :icon="Delete" aria-label="删除实例" title="删除" @click="handleDelete(row)" v-permission="['datasource:delete']" /></template></el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div class="pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next" @current-change="fetchData" @size-change="handleSearch" /></div>
    </el-card>
    <el-drawer v-model="pickerVisible" class="datasource-drawer" direction="rtl" size="min(320px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header="{ titleId, titleClass }">
        <div class="drawer-header">
          <DrawerBackButton @click="pickerVisible = false" />
          <span :id="titleId" :class="[titleClass, 'drawer-title']">选择数据源类型</span>
        </div>
      </template>
      <div class="type-groups">
        <section v-for="group in groupedTypeOptions" :key="group.value" class="type-group">
          <div class="type-group-title">{{ group.label }}</div>
          <div class="type-grid">
            <button v-for="item in group.items" :key="item.type" type="button" class="type-card" :aria-label="`接入 ${item.name}`" @click="selectDatasourceType(item)">
              <img v-if="item.type === 'prometheus'" :src="prometheusLogo" alt="" class="type-card-logo" />
              <img v-else-if="item.type === 'victoriametrics'" :src="victoriaMetricsLogo" alt="" class="type-card-logo vm-logo" />
              <img v-else-if="item.type === 'loki'" :src="lokiLogo" alt="" class="type-card-logo loki-logo" />
              <img v-else-if="item.type === 'elasticsearch'" :src="elasticsearchLogo" alt="" class="type-card-logo log-source-logo" />
              <img v-else-if="item.type === 'clickhouse'" :src="clickhouseLogo" alt="" class="type-card-logo log-source-logo" />
              <img v-else-if="item.type === 'jaeger'" :src="jaegerLogo" alt="" class="type-card-logo jaeger-logo" />
              <el-icon v-else :size="26" color="#409eff"><Connection /></el-icon>
              <span class="type-card-name">{{ item.name }}</span>
              <span class="type-card-desc">{{ sourceDescription(item) }}</span>
            </button>
          </div>
        </section>
        <el-empty v-if="!datasourceTypes.length" description="暂无可用的数据源类型" :image-size="72" />
      </div>
      <template #footer><el-button @click="pickerVisible = false">取消</el-button></template>
    </el-drawer>
    <el-drawer v-model="drawerVisible" class="datasource-drawer" direction="rtl" size="min(460px, 94vw)" :show-close="false" :close-on-click-modal="false">
      <template #header="{ titleId, titleClass }">
        <div class="drawer-header">
          <DrawerBackButton :disabled="saving || testing" @click="handleDrawerBack" />
          <span :id="titleId" :class="[titleClass, 'drawer-title']">{{ form.id ? '编辑接入' : '新增接入' }}</span>
        </div>
      </template>
      <div class="step2-wrapper">
        <div class="selected-type-banner">
          <div class="selected-type-icon" :style="{ background: form.type === 'prometheus' ? '#fff3eb' : (['victoriametrics', 'loki', 'elasticsearch', 'clickhouse', 'jaeger'].includes(form.type) ? 'transparent' : '#ecf5ff') }">
            <img v-if="form.type === 'prometheus'" :src="prometheusLogo" alt="" class="selected-type-logo" />
            <img v-else-if="form.type === 'victoriametrics'" :src="victoriaMetricsLogo" alt="" class="selected-type-logo vm-logo" />
            <img v-else-if="form.type === 'loki'" :src="lokiLogo" alt="" class="selected-type-logo loki-logo" />
            <img v-else-if="form.type === 'elasticsearch'" :src="elasticsearchLogo" alt="" class="selected-type-logo log-source-logo" />
            <img v-else-if="form.type === 'clickhouse'" :src="clickhouseLogo" alt="" class="selected-type-logo log-source-logo" />
            <img v-else-if="form.type === 'jaeger'" :src="jaegerLogo" alt="" class="selected-type-logo jaeger-logo" />
            <el-icon v-else :size="22" color="#409eff"><Connection /></el-icon>
          </div>
          <div class="selected-type-text">
            <div class="selected-type-label">{{ selectedType.name }}</div>
            <div class="selected-type-desc">{{ sourceGroupLabel(sourceGroup(selectedType)) }} / {{ sourceDescription(selectedType) }}</div>
          </div>
          <el-tag size="small" effect="plain">已选择</el-tag>
        </div>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="config-form" size="large">
        <el-form-item label="实例名称" prop="name"><el-input v-model="form.name" maxlength="100" :placeholder="['prometheus', 'victoriametrics', 'loki', 'elasticsearch', 'clickhouse', 'jaeger'].includes(form.type) ? '' : '例如：生产环境监控'" /></el-form-item>
        <el-form-item label="接入地址" prop="url"><el-input v-model="form.url" :placeholder="['prometheus', 'victoriametrics', 'loki', 'elasticsearch', 'clickhouse', 'jaeger'].includes(form.type) ? '' : selectedType.url_placeholder" /><p v-if="!['prometheus', 'victoriametrics', 'loki', 'elasticsearch', 'clickhouse', 'jaeger'].includes(form.type)" class="field-help">{{ selectedType.url_help }}</p></el-form-item>
        <el-form-item v-if="form.type !== 'clickhouse'" label="认证方式"><div class="auth-segmented" role="group" aria-label="认证方式"><button v-for="option in authOptions" :key="option.value" type="button" class="auth-seg" :class="{ active: form.auth_type === option.value }" :aria-pressed="form.auth_type === option.value" @click="form.auth_type = option.value">{{ option.label }}</button></div></el-form-item>
        <template v-if="form.auth_type === 'basic'"><el-form-item label="用户名"><el-input v-model="authFields.username" autocomplete="off" /></el-form-item><el-form-item label="密码"><el-input v-model="authFields.password" type="password" show-password :placeholder="canReuseAuth ? '留空保留已保存的密码' : ''" /></el-form-item></template>
        <el-form-item v-if="['bearer', 'api_key'].includes(form.auth_type)" :label="form.auth_type === 'api_key' ? 'API Key' : 'Token'"><el-input v-model="authFields.token" type="password" show-password :placeholder="canReuseAuth ? (form.auth_type === 'api_key' ? '留空保留已保存的 API Key' : '留空保留已保存的 Token') : ''" /></el-form-item>
        <el-form-item v-if="form.type === 'clickhouse'" label="数据库" required><el-input v-model="logConfig.database" maxlength="128" /></el-form-item>
        <el-form-item v-if="form.type === 'clickhouse'" label="启用日志字段映射"><el-switch v-model="logMappingEnabled" /></el-form-item>
        <template v-if="hasLogMapping">
          <el-divider content-position="left">日志查询配置</el-divider>
          <el-form-item v-if="form.type === 'elasticsearch'" label="索引 / 索引模式" required><el-input v-model="logConfig.index" maxlength="512" /></el-form-item>
          <template v-if="form.type === 'clickhouse'">
            <el-form-item label="日志表" required><el-input v-model="logConfig.table" maxlength="128" /></el-form-item>
          </template>
          <el-form-item label="时间字段" required><el-input v-model="logConfig.timestamp_field" maxlength="128" /></el-form-item>
          <el-form-item label="日志正文字段" required><el-input v-model="logConfig.message_field" maxlength="128" /></el-form-item>

        </template>
        <el-form-item label="标签"><div class="kv-editor"><el-button :icon="Plus" plain :disabled="form.labels.length >= 20" @click="form.labels.push({ key: '', value: '' })">添加标签</el-button><div v-for="(row, index) in form.labels" :key="index" class="kv-row"><el-input v-model="row.key" placeholder="Key" maxlength="64" /><el-input v-model="row.value" placeholder="Value" maxlength="200" /><el-button :icon="Minus" circle aria-label="删除标签" @click="form.labels.splice(index, 1)" /></div></div></el-form-item>
        <el-collapse class="advanced-config"><el-collapse-item title="高级配置" name="advanced"><template v-if="hasLogMapping">          <el-form-item label="服务字段"><el-input v-model="logConfig.service_field" maxlength="128" /></el-form-item>
          <el-form-item label="级别字段"><el-input v-model="logConfig.level_field" maxlength="128" /></el-form-item>
          <el-form-item label="Trace ID 字段"><el-input v-model="logConfig.trace_id_field" maxlength="128" /></el-form-item></template><el-form-item label="自定义 HTTP 头"><div class="kv-editor"><el-button :icon="Plus" plain :disabled="headers.length >= 20" @click="headers.push({ key: '', value: '' })">添加 HTTP 头</el-button><div v-for="(row, index) in headers" :key="index" class="kv-row"><el-input v-model="row.key" placeholder="Header 名称" maxlength="128" /><el-input v-model="row.value" placeholder="Header 值" maxlength="1000" /><el-button :icon="Minus" circle aria-label="删除 HTTP 头" @click="headers.splice(index, 1)" /></div></div></el-form-item></el-collapse-item></el-collapse>
        <el-form-item label="备注"><el-input v-model="form.remark" type="textarea" :rows="2" maxlength="500" /></el-form-item>
        <el-form-item label="启用状态"><el-switch v-model="form.status" :active-value="1" :inactive-value="0" /></el-form-item>
      </el-form>
      </div>
      <template #footer><div class="form-footer"><el-button class="btn-test" :loading="testing" @click="testFromForm">测试连接</el-button><el-button @click="drawerVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="handleSubmit">保存</el-button></div></template>
    </el-drawer>
    <el-dialog v-model="testVisible" title="连接测试结果" width="min(440px, 94vw)"><el-result v-if="testResult" :icon="testResult.ok ? 'success' : 'error'" :title="testResult.ok ? '连接成功' : '连接失败'" :sub-title="testResult.message" /><template #footer><el-button @click="testVisible = false">关闭</el-button></template></el-dialog>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, ArrowLeft, Connection, Delete, Edit, Minus, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { getDatasourceTypes, getDatasources, createDatasource, updateDatasource, deleteDatasource, batchUpdateDatasourceStatus, batchDeleteDatasources, testConnection, testSavedConnection } from '../../api/datasource'
import jaegerLogo from '../../assets/jaeger-logo.svg'
import elasticsearchLogo from '../../assets/elasticsearch-logo.svg'
import clickhouseLogo from '../../assets/clickhouse-logo.svg'
import lokiLogo from '../../assets/loki-logo.svg'
import victoriaMetricsLogo from '../../assets/victoriametrics-logo.svg'
import prometheusLogo from '../../assets/prometheus-logo.7aa022e5.svg'

const props = defineProps({ groupId: { type: Number, default: null }, groupName: { type: String, default: '' }, refreshKey: { type: Number, default: 0 }, requireGroup: { type: Boolean, default: false } })
const emit = defineEmits(['changed'])
const creationGroupId = ref(null)
watch(() => [props.groupId, props.refreshKey], () => { selectedRows.value = []; handleSearch() })

const loading = ref(false), saving = ref(false), testing = ref(false), drawerVisible = ref(false), formRef = ref()
const tableData = ref([]), selectedRows = ref([]), keyword = ref(''), statusFilter = ref(''), typeFilter = ref(''), batchAction = ref('')
const page = ref(1), pageSize = ref(10), total = ref(0), sortBy = ref('id'), sortDesc = ref(false)
const testVisible = ref(false), testResult = ref(null), headers = ref([])
const datasourceTypes = ref([])
const pickerVisible = ref(false)
const sourceDescription = item => ({ jaeger: 'Trace 链路追踪', loki: '日志查询', elasticsearch: '全文日志检索', clickhouse: '列式分析数据库', prometheus: '指标监控', victoriametrics: '时序数据库' }[item.type] || capabilityLabel(item.capabilities))
const baseAuthOptions = [{ value: 'none', label: '无认证' }, { value: 'basic', label: 'Basic Auth' }, { value: 'bearer', label: 'Bearer Token' }]

const sourceGroup = item => {
  if (item.category === 'database') return 'database'
  if (item.capabilities?.includes('logs')) return 'logs'
  if (item.capabilities?.includes('traces')) return 'traces'
  return item.category || 'observability'
}
const sourceGroupLabel = value => ({ observability: '监控', logs: '日志', database: '数据库', traces: '链路' }[value] || value)
const sourceGroupOrder = ['observability', 'logs', 'database', 'traces']
const groupedTypeOptions = computed(() => {
  const groups = new Map()
  for (const item of datasourceTypes.value) {
    const value = sourceGroup(item)
    if (!groups.has(value)) groups.set(value, { value, label: sourceGroupLabel(value), items: [] })
    groups.get(value).items.push(item)
  }
  return [...groups.values()].sort((a, b) => {
    const order = value => sourceGroupOrder.includes(value) ? sourceGroupOrder.indexOf(value) : sourceGroupOrder.length
    return order(a.value) - order(b.value)
  })
})
function handleDrawerBack() {
  drawerVisible.value = false
  if (!form.id) pickerVisible.value = true
}
const typeDefinition = type => datasourceTypes.value.find(item => item.type === type) || {}
const selectedType = computed(() => typeDefinition(form.type))
const capabilityLabel = capabilities => (capabilities || []).map(value => ({ metrics: '指标查询与告警规则评估', logs: '日志查询', traces: '链路查询', sql: 'SQL 查询' }[value] || value)).join('、')
const blankForm = () => ({ id: null, name: '', type: '', category: 'observability', capabilities: 'metrics', url: '', auth_type: 'none', labels: [], config: '', tunnel_id: 0, priority: 100, region: '', env: '', domain: '', remark: '', status: 1 })
const defaultLogConfig = type => ({ index: type === 'elasticsearch' ? 'logs-*' : '', database: 'default', table: 'otel_logs', timestamp_field: type === 'clickhouse' ? 'Timestamp' : '@timestamp', message_field: type === 'clickhouse' ? 'Body' : 'message', service_field: '', level_field: '', trace_id_field: '' })
const logConfig = reactive(defaultLogConfig(''))
const logMappingEnabled = ref(false)
const hasLogMapping = computed(() => form.type === 'elasticsearch' || (form.type === 'clickhouse' && logMappingEnabled.value))
const form = reactive(blankForm()), authFields = reactive({ username: '', password: '', token: '' })
let originalAuth = {}, originalAuthType = 'none', originalConfig = {}, keywordTimer, requestID = 0
const authOptions = computed(() => {
  if (form.type === 'clickhouse') return baseAuthOptions.filter(option => option.value !== 'bearer')
  if (form.type === 'elasticsearch') return [...baseAuthOptions, { value: 'api_key', label: 'API Key' }]
  return baseAuthOptions
})
const canReuseAuth = computed(() => Boolean(form.id && form.auth_type === originalAuthType))
const rules = {
  type: [{ required: true, message: '请选择数据源类型', trigger: 'change' }],
  name: [{ required: true, whitespace: true, message: '请输入实例名称', trigger: 'blur' }],
  url: [{ validator: (_r, value, done) => { try { const url = new URL(value); if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || url.search || url.hash || /\s/.test(value)) throw new Error(); done() } catch { done(new Error('请输入完整的 HTTP/HTTPS 地址，认证信息请单独配置')) } }, trigger: 'blur' }],
}
onMounted(async () => {
  const response = await getDatasourceTypes()
  datasourceTypes.value = response.data || []
  await fetchData()
})
onBeforeUnmount(() => { clearTimeout(keywordTimer); requestID++ })
async function fetchData() {
  const current = ++requestID
  if (props.requireGroup && !props.groupId) {
    tableData.value = []; total.value = 0; selectedRows.value = []; loading.value = false
    return
  }
  loading.value = true
  try {
    const response = await getDatasources({ group_id: props.groupId || undefined, page: page.value, page_size: pageSize.value, keyword: keyword.value, status: statusFilter.value, type: typeFilter.value, sort_by: sortBy.value, sort_desc: sortDesc.value })
    if (current !== requestID) return
    tableData.value = response.data.list || []; total.value = response.data.total || 0; selectedRows.value = []
    const lastPage = Math.max(1, Math.ceil(total.value / pageSize.value))
    if (page.value > lastPage) { page.value = lastPage; return fetchData() }
  } finally { if (current === requestID) loading.value = false }
}
function handleSearch() { clearTimeout(keywordTimer); page.value = 1; fetchData() }
function handleKeywordInput() { clearTimeout(keywordTimer); keywordTimer = setTimeout(handleSearch, 300) }
function handleReset() { keyword.value = ''; statusFilter.value = ''; typeFilter.value = ''; handleSearch() }
function handleSortChange({ prop, order }) { sortBy.value = prop || 'id'; sortDesc.value = order === 'descending'; handleSearch() }
function parseObject(value) { if (value && typeof value === 'object') return value; try { return JSON.parse(value || '{}') || {} } catch { return {} } }
function objectToRows(value) { return Object.entries(value).map(([key, value]) => ({ key, value })) }
function rowsToObject(rows) { return Object.fromEntries(rows.filter(row => row.key.trim()).map(row => [row.key.trim(), row.value])) }
function handleAdd() {
  if (props.requireGroup && !props.groupId) return
  creationGroupId.value = props.groupId
  pickerVisible.value = true
  Object.assign(form, blankForm()); Object.assign(authFields, { username: '', password: '', token: '' }); headers.value = []
  originalAuth = {}; originalAuthType = 'none'; originalConfig = {}; formRef.value?.clearValidate(); drawerVisible.value = false
}
function selectDatasourceType(item) {
  if (form.type !== item.type) {
    Object.assign(form, blankForm(), { type: item.type, auth_type: item.type === 'clickhouse' ? 'basic' : 'none' })
    Object.assign(logConfig, defaultLogConfig(item.type))
    logMappingEnabled.value = false
    originalConfig = {}
    Object.assign(authFields, { username: item.type === 'clickhouse' ? 'default' : '', password: '', token: '' })
    headers.value = []
  }
  pickerVisible.value = false
  drawerVisible.value = true
  formRef.value?.clearValidate()
}
function handleEdit(row) {
  pickerVisible.value = false
  drawerVisible.value = true
  Object.assign(form, blankForm(), row, { labels: objectToRows(parseObject(row.labels)) })
  originalAuth = parseObject(row.auth_config); originalAuthType = row.auth_type || 'none'; originalConfig = parseObject(row.config)
  if (form.type === 'clickhouse') form.auth_type = 'basic'
  Object.assign(logConfig, defaultLogConfig(row.type), originalConfig.log_query || {})
  if (form.type === 'clickhouse') logConfig.database = originalConfig.database || originalConfig.log_query?.database || 'default'
  logMappingEnabled.value = Boolean(originalConfig.log_query)
  Object.assign(authFields, { username: originalAuth.username || (form.type === 'clickhouse' ? 'default' : ''), password: '', token: '' }); headers.value = objectToRows(originalConfig.custom_headers || {})
  formRef.value?.clearValidate(); drawerVisible.value = true
}
function buildAuthConfig() {
  if (form.auth_type === 'none') return ''
  if (form.auth_type === 'basic') {
    if (canReuseAuth.value && !authFields.password && authFields.username === originalAuth.username) return ''
    return JSON.stringify({ username: authFields.username.trim(), password: authFields.password || (canReuseAuth.value ? (originalAuth.password || '') : '') })
  }
  if (canReuseAuth.value && !authFields.token) return ''
  return JSON.stringify({ token: authFields.token.trim() })
}
async function payload() {
  if (!await formRef.value.validate().catch(() => false)) return null
  if (form.auth_type === 'basic' && (!authFields.username.trim() || (form.type !== 'clickhouse' && !authFields.password && !(canReuseAuth.value && originalAuth.password)))) { ElMessage.warning('请填写用户名和密码'); return null }
  if (['bearer', 'api_key'].includes(form.auth_type) && !authFields.token.trim() && !(canReuseAuth.value && originalAuth.token)) { ElMessage.warning(form.auth_type === 'api_key' ? '请填写 API Key' : '请填写 Bearer Token'); return null }
  for (const [rows, label] of [[form.labels, '标签'], [headers.value, 'HTTP 头']]) {
    const keys = new Set()
    for (const row of rows) {
      const key = row.key.trim(), normalized = label === 'HTTP 头' ? key.toLowerCase() : key
      if (!key || keys.has(normalized)) { ElMessage.warning(`${label}名称不能为空或重复`); return null }
      if (label === 'HTTP 头' && (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(key) || /[\r\n]/.test(row.value))) { ElMessage.warning('HTTP 头格式不正确'); return null }
      keys.add(normalized)
    }
  }
  const config = { ...originalConfig, custom_headers: rowsToObject(headers.value) }
  if (form.type === 'clickhouse') {
    if (!logConfig.database.trim()) { ElMessage.warning('请填写数据库'); return null }
    config.database = logConfig.database.trim()
  }
  if (hasLogMapping.value) {
    const mapping = Object.fromEntries(Object.entries(logConfig).map(([key, value]) => [key, value.trim()]))
    const required = ['timestamp_field', 'message_field', ...(form.type === 'elasticsearch' ? ['index'] : ['database', 'table'])]
    if (required.some(key => !mapping[key])) { ElMessage.warning('请填写日志索引或数据库、表以及时间和正文字段'); return null }
    config.log_query = mapping
  } else if (form.type === 'clickhouse') { delete config.log_query }
  return { ...form, name: form.name.trim(), type: form.type, url: form.url.trim(), labels: JSON.stringify(rowsToObject(form.labels)), auth_config: buildAuthConfig(), config: JSON.stringify(config) }
}
async function handleSubmit() {
  const data = await payload(); if (!data) return
  saving.value = true
  try { form.id ? await updateDatasource(form.id, data) : await createDatasource({ ...data, group_id: creationGroupId.value || undefined }); ElMessage.success('实例已保存'); drawerVisible.value = false; emit('changed'); await fetchData() } finally { saving.value = false }
}
async function testFromForm() {
  const data = await payload(); if (!data) return
  testing.value = true
  try {
    const response = form.id ? await testSavedConnection(form.id, data) : await testConnection(data)
    testResult.value = { ok: true, message: response.message }
  } catch (error) { testResult.value = { ok: false, message: error.response?.data?.message || error.message || '连接测试失败' } }
  finally { testing.value = false; testVisible.value = true }
}
async function handleDelete(row) {
  if (!await ElMessageBox.confirm(`确定删除实例「${row.name}」吗？`, '删除实例', { type: 'warning' }).then(() => true).catch(() => false)) return
  await deleteDatasource(row.id); ElMessage.success('实例已删除'); emit('changed'); await fetchData()
}
async function handleBatchCommand(command) {
  if (!selectedRows.value.length) return
  const ids = selectedRows.value.map(row => row.id)
  if (command === 'delete' && !await ElMessageBox.confirm(`确定删除选中的 ${ids.length} 个实例吗？`, '批量删除', { type: 'warning' }).then(() => true).catch(() => false)) return
  batchAction.value = command
  try { command === 'delete' ? await batchDeleteDatasources(ids) : await batchUpdateDatasourceStatus(ids, command === 'enable' ? 1 : 0); ElMessage.success('批量操作完成'); emit('changed'); await fetchData() } finally { batchAction.value = '' }
}
</script>

<style scoped>
.datasource-toolbar{display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap}.datasource-filters,.datasource-actions{display:flex;align-items:center;gap:8px}.datasource-filters{flex:1;min-width:0}.filter-keyword{width:280px;min-width:180px}.filter-status{width:120px}.filter-type{width:180px}.batch-arrow{margin-left:6px}.instance-name{display:inline-flex;align-items:center;gap:9px}.instance-name img{width:20px;height:20px}.scope-tags{display:flex;gap:6px;flex-wrap:wrap}.status{display:inline-flex;align-items:center;gap:7px}.status i{width:7px;height:7px;border-radius:50%;background:var(--el-color-info)}.status i.enabled{background:var(--el-color-success)}.pagination{display:flex;justify-content:flex-end;margin-top:16px}.source-banner{display:flex;align-items:center;gap:14px;padding:16px;margin-bottom:22px;border:1px solid var(--ops-border);border-radius:8px;background:var(--ops-surface-soft)}.source-banner img{width:34px;height:34px}.source-banner strong{font-size:16px}.source-banner p{margin:5px 0 0;color:var(--ops-muted);font-size:12px}.kv-editor{display:flex;width:100%;flex-direction:column;gap:8px}.kv-editor>.el-button{border-style:dashed}.kv-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr) 32px;gap:8px}.advanced-config{margin-bottom:20px}.field-help{font-size:12px;color:var(--ops-muted)}.form-footer{display:flex;align-items:center;gap:8px}.test-button{margin-right:auto}@media(max-width:900px){.datasource-filters{flex-basis:100%}.datasource-actions{margin-left:auto}}@media(max-width:600px){.datasource-filters{flex-wrap:wrap}.filter-keyword{width:100%}.pagination{overflow-x:auto;justify-content:flex-start}}
/* ── 步骤 1：类型选择卡片 ── */
.type-groups {
  display: flex;
  flex-direction: column;
  gap: 18px;
}
.type-group-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--ops-text);
  margin-bottom: 10px;
}
.type-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  justify-content: start;
  gap: 8px;
}
.type-card {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 12px 6px;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  box-sizing: border-box;
  min-width: 0;
  aspect-ratio: 1;
  min-height: 0;
}
.type-card:hover,
.type-card.active {
  border-color: var(--ops-primary);
  background-color: var(--ops-primary-soft);
  transform: translateY(-1px);
  box-shadow: 0 2px 8px rgba(64, 158, 255, 0.15);
}
.type-card-name {
  font-size: 12px;
  max-width: 100%;
  overflow-wrap: anywhere;
  font-weight: 500;
  margin-top: 8px;
}
.type-card-desc {
  font-size: 11px;
  color: var(--ops-muted);
  margin-top: 4px;
}
.type-card-logo {
  width: 26px;
  height: 26px;
  display: block;
}
.type-card-logo-wide {
  width: 76px;
  height: 26px;
  object-fit: contain;
}

/* ── 步骤 2：表单 ── */
/* 已选类型横幅 */
.selected-type-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  margin-bottom: 16px;
  background: #f8fafc;
  border: 1px solid var(--ops-border);
  border-radius: 10px;
}
.selected-type-icon {
  width: 42px;
  height: 42px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 10px;
  flex-shrink: 0;
}
.selected-type-icon-wide {
  width: 86px;
}
.selected-type-logo {
  width: 24px;
  height: 24px;
  display: block;
}
.selected-type-logo-wide {
  width: 76px;
  height: 18px;
  object-fit: contain;
}
.selected-type-label {
  font-size: 15px;
  font-weight: 600;
  color: var(--ops-text);
}
.selected-type-desc {
  font-size: 12px;
  color: var(--ops-muted);
  margin-top: 2px;
}

/* 表单容器 */
.config-form {
  display: flex;
  flex-direction: column;
  gap: 0;
}

/* 扁平表单区块 */
.section-card {
  background: transparent;
}
.section-body {
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.label-editor { display: flex; flex-direction: column; gap: 8px; }
.add-label-button { width: 100%; border-style: dashed; }
.label-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 28px; align-items: center; gap: 8px; }
.label-row .label-icon-button {
  width: 26px;
  height: 26px;
  color: #8a94a3;
  background: var(--ops-surface);
  border-color: var(--ops-border);
}
.label-row .label-icon-button :deep(.el-icon) { font-size: 14px; }
.label-row .label-icon-button:hover,
.label-row .label-icon-button:focus-visible {
  color: #606b7b;
  background: var(--ops-surface-soft);
  border-color: #c8cdd5;
}
.field-help { margin-top: 7px; color: var(--ops-muted); font-size: 12px; line-height: 1.5; }

/* 表单标签（放在 el-form-item 上方） */
.form-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ops-muted);
  margin-bottom: 6px;
  margin-top: 4px;
}
.form-label .required {
  color: var(--ops-danger);
  margin-left: 2px;
}

/* 认证方式 —— 分段选择器 */
.auth-segmented {
  display: flex;
  width: 100%;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  overflow: hidden;
  background: var(--ops-surface-soft);
}
.auth-seg {
  flex: 1;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  padding: 8px 14px;
  border: none;
  background: transparent;
  font-size: 13px;
  color: var(--ops-muted);
  cursor: pointer;
  transition: all 0.2s;
  outline: none;
  white-space: nowrap;
  font-family: inherit;
}
.auth-seg + .auth-seg {
  border-left: 1px solid var(--ops-border);
}
.auth-seg:hover {
  color: var(--ops-primary);
  background: rgba(64,158,255,0.04);
}
.auth-seg.active {
  background: var(--ops-primary);
  color: #fff;
}

/* 认证字段过渡 */
.auth-fields {
  padding-top: 4px;
}

.advanced-config {
  margin-top: 4px;
  overflow: hidden;
  border: 1px solid var(--ops-border);
  border-radius: 8px;
}
.advanced-config :deep(.el-collapse-item__header) {
  height: 48px;
  padding: 0 14px;
  color: #4b5565;
  border-bottom: 0;
}
.advanced-config :deep(.el-collapse-item__wrap) { border-bottom: 0; }
.advanced-config :deep(.el-collapse-item__content) { padding: 0 14px 16px; }
.advanced-title { display: inline-flex; align-items: center; gap: 8px; font-weight: 600; }
.advanced-content { display: flex; flex-direction: column; gap: 14px; }
.advanced-heading { color: var(--ops-text); font-size: 13px; font-weight: 600; }
.advanced-heading span { color: var(--ops-muted); font-weight: 400; }
.header-editor { display: flex; flex-direction: column; gap: 8px; }
.header-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 28px; align-items: center; gap: 8px; }
.header-remove-button {
  width: 26px;
  height: 26px;
  color: #8a94a3;
  background: var(--ops-surface);
  border-color: var(--ops-border);
}
.header-remove-button:hover,
.header-remove-button:focus-visible {
  color: #606b7b;
  background: var(--ops-surface-soft);
  border-color: #c8cdd5;
}
.add-header-button { width: 100%; border-style: dashed; }

/* 其他行 */
.extra-row {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.extra-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.extra-item.inline {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
}
.extra-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--ops-muted);
}

/* 表单底部 */
.form-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
}
.btn-test {
  margin-right: auto;
  border-color: #67C23A;
  color: #67C23A;
  background: #f0f9eb;
}
.btn-test:hover {
  border-color: #67C23A;
  color: #fff;
  background: #67C23A;
}


:global(.el-drawer.datasource-drawer) { border-radius:0; }
.drawer-header { display:flex; align-items:center; gap:12px; }
.drawer-title { font-size:16px; font-weight:500; color:var(--ops-text); }
.back-button { padding:8px 6px; }
.type-card { background:var(--el-bg-color); font-family:inherit; color:var(--el-text-color-primary); }
.type-card:focus-visible,.auth-seg:focus-visible { outline:2px solid var(--ops-primary); outline-offset:3px; }
.selected-type-text { flex:1; min-width:0; }
.config-form :deep(.el-form-item__label) { font-size:13px; font-weight:500; color:var(--ops-muted); }
.config-form .kv-editor>.el-button { width:100%; }
.picker-footer { display:flex; justify-content:flex-end; }
@media(prefers-reduced-motion:reduce) { .type-card { transition:none; } }
.log-source-logo { object-fit: contain; }
.jaeger-logo { object-fit: contain; }
.loki-logo { object-fit: contain; }
.vm-logo { object-fit:contain; filter:invert(1); }
html.dark .vm-logo { filter:none; }
.selected-type-logo { object-fit:contain; }
</style>

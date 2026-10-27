<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="通知模板筛选">
        <el-input v-model="keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索通知模板" placeholder="搜索名称或描述" @input="handleFilterChange" @clear="applyFilters" />
        <el-select v-model="typeFilter" clearable aria-label="按模板类型筛选" placeholder="模板类型" @change="applyFilters"><el-option v-for="item in templateTypes" :key="item.value" :label="item.label" :value="item.value" /></el-select>
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadTemplates">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openDrawer()" v-permission="['notification:template:edit']">
          创建
        </el-button>
      </div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="pagedTemplates" v-loading="loading" row-key="id" stripe class="template-table">
        <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
        <el-table-column label="模板类型" width="100" align="center">
          <template #default="{ row }">
            <span class="type-cell" :title="templateTypeMeta(row.type).label">
              <img v-if="templateTypeMeta(row.type).icon" :src="templateTypeMeta(row.type).icon" :alt="templateTypeMeta(row.type).label" />
              <span v-else>{{ templateTypeMeta(row.type).label }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="76" fixed="right" align="center">
          <template #default="{ row }">
            <div class="row-actions">
              <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑通知模板" @click="openDrawer(row)" v-permission="['notification:template:edit']" /></el-tooltip>
              <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除通知模板" @click="removeOne(row)" v-permission="['notification:template:edit']" /></el-tooltip>
            </div>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div v-if="filteredTemplates.length > 0" class="task-pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="filteredTemplates.length" :page-sizes="[10, 20, 50, 100]" layout="total, sizes, prev, pager, next" /></div>
    </el-card>

    <el-drawer v-model="drawerVisible" class="standard-config-drawer template-drawer" size="min(460px, 94vw)" direction="rtl" :close-on-click-modal="false">
      <template #header>
        <div class="drawer-header">
          <DrawerBackButton @click="drawerVisible = false" />
          <span class="drawer-title">{{ form.id ? '编辑通知模板' : '创建通知模板' }}</span>
        </div>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="drawer-form">
        <div class="form-row">
          <el-form-item label="名称" prop="name">
            <el-input v-model="form.name" />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="form.description" />
          </el-form-item>
        </div>

        <el-form-item label="模板类型" prop="type">
          <div class="type-card-list">
            <button
              v-for="item in templateTypes"
              :key="item.value"
              class="type-card"
              :class="{ active: form.type === item.value }"
              type="button"
              @click="form.type = item.value"
            >
              <span class="type-option">
                <img :src="item.icon" :alt="item.label" />
                <span>{{ item.label }}</span>
              </span>
            </button>
          </div>
        </el-form-item>

        <section v-if="form.type === 'feishu'" class="template-notice" role="note" aria-label="飞书通知模板说明">
          <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
          <span class="intro-message"><strong>只需编辑 Markdown 正文。</strong>标题、标准表情符号、颜色和页脚由系统固定生成。</span>
          <div class="color-legend" aria-label="告警卡片颜色说明">
            <span class="p0">P0</span><span class="p1">P1</span><span class="p2">P2</span><span class="recovered">已恢复</span>
          </div>
        </section>

        <details class="template-fields" open>
          <summary>可用字段</summary>
          <div class="field-grid">
            <div v-for="item in templateFields" :key="item.name" class="field-item">
              <code>{{ item.name }}</code><span>{{ item.description }}</span>
            </div>
          </div>
        </details>

        <el-form-item prop="content">
          <template #label>
            <span class="template-field-label">Markdown 内容<el-tooltip content="支持 {{ if }}、{{ with }} 和 {{ range .alerts }}；可使用告警 JSON 中的 alertname、severity、value、labels、annotations 等字段。" placement="top"><el-icon class="help-icon" tabindex="0" aria-label="Markdown 变量说明"><QuestionFilled /></el-icon></el-tooltip></span>
          </template>
          <el-input v-model="form.content" type="textarea" :rows="22" class="template-editor" />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-footer">
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Delete, Edit, InfoFilled, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import feishuIcon from '../../assets/feishu.svg'
import emailIcon from '../../assets/email.svg'
import dingdingIcon from '../../assets/dingding.svg'
import qywechatIcon from '../../assets/qywechat.svg'
import {
  createNotificationTemplate,
  deleteNotificationTemplate,
  getNotificationTemplates,
  updateNotificationTemplate,
} from '../../api/notification'

const templateTypes = [
  { value: 'qywechat', label: '企业微信', icon: qywechatIcon },
  { value: 'feishu', label: '飞书', icon: feishuIcon },
  { value: 'email', label: '邮件', icon: emailIcon },
  { value: 'dingding', label: '钉钉', icon: dingdingIcon },
]

const templateFields = [
  { name: '.alertname', description: '告警名称' },
  { name: '.severity', description: '告警等级' },
  { name: '.status', description: '当前状态' },
  { name: '.value', description: '当前值' },
  { name: '.datasource', description: '数据源' },
  { name: '.fingerprint', description: '告警指纹' },
  { name: '.first_trigger_time_format', description: '触发时间' },
  { name: '.recover_time_format', description: '恢复时间' },
  { name: '.labels', description: 'PromQL 标签' },
  { name: '.datasource_labels', description: '数据源标签' },
  { name: '.external_labels', description: '额外标签' },
  { name: '.annotations', description: '描述与注解' },
  { name: '.alerts', description: '分组告警成员数组' },
  { name: '.alerts[].first_trigger_time_format', description: '分组成员触发时间' },
  { name: '.alerts[].recover_time_format', description: '分组成员恢复时间' },
]

const defaultTemplate = `{{- if .alerts }}
**📋 分组告警：** {{ len .alerts }} 条
{{- range .alerts }}
- [{{ .severity }}] {{ .alertname }}{{ with .labels.instance }} · {{ . }}{{ end }}
  {{- with .first_trigger_time_format }}
  - 触发时间：{{ . }}
  {{- end }}
  {{- with .recover_time_format }}
  - 恢复时间：{{ . }}
  {{- end }}
{{- end }}
{{- else }}
**告警名称：** {{ .alertname }}
**告警等级：** {{ .severity }}
{{- with .value }}
**当前值：** {{ . }}
{{- end }}
{{- with .annotations.description }}
**描述信息：** {{ . }}
{{- end }}
{{- with .datasource }}
**数据源：** {{ . }}
{{- end }}
{{- with .labels.instance }}
**实例：** {{ . }}
{{- end }}
{{- with .first_trigger_time_format }}
**触发时间：** {{ . }}
{{- end }}
{{- with .recover_time_format }}
**恢复时间：** {{ . }}
{{- end }}
{{- end }}`

const templates = ref([])
const loading = ref(false)
const saving = ref(false)
const drawerVisible = ref(false)
const formRef = ref()
const keyword = ref('')
const typeFilter = ref('')
const page = ref(1)
const pageSize = ref(10)
let filterTimer

const hasFilters = computed(() => keyword.value.trim() !== '' || typeFilter.value !== '')
const filteredTemplates = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  return templates.value.filter(item => (!term || [item.name, item.description].some(value => String(value || '').toLowerCase().includes(term))) && (!typeFilter.value || item.type === typeFilter.value))
})
const pagedTemplates = computed(() => filteredTemplates.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))

const form = reactive({
  id: null,
  name: '',
  description: '',
  type: 'feishu',
  advanced_card: true,
  content: defaultTemplate,
})

const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  content: [{ required: true, message: '请输入 Markdown 内容', trigger: 'blur' }],
}

onMounted(loadTemplates)
onBeforeUnmount(() => clearTimeout(filterTimer))

function handleFilterChange() { clearTimeout(filterTimer); filterTimer = setTimeout(applyFilters, 300) }
function applyFilters() { page.value = 1 }
function clearFilters() { keyword.value = ''; typeFilter.value = ''; applyFilters() }

async function loadTemplates() {
  loading.value = true
  try {
    const res = await getNotificationTemplates()
    templates.value = res.data || []
    page.value = Math.min(page.value, Math.max(1, Math.ceil(filteredTemplates.value.length / pageSize.value)))
  } finally {
    loading.value = false
  }
}

function openDrawer(row) {
  Object.assign(form, row ? {
    id: row.id,
    name: row.name,
    description: row.description || '',
    type: row.type || 'feishu',
    advanced_card: !!row.advanced_card,
    content: row.content || defaultTemplate,
  } : {
    id: null,
    name: '',
    description: '',
    type: 'feishu',
    advanced_card: true,
    content: defaultTemplate,
  })
  drawerVisible.value = true
}

async function save() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = { ...form, advanced_card: form.type === 'feishu', status: 1 }
    if (form.id) await updateNotificationTemplate(form.id, payload)
    else await createNotificationTemplate(payload)
    ElMessage.success('通知模板已保存')
    drawerVisible.value = false
    await loadTemplates()
  } finally {
    saving.value = false
  }
}

async function removeOne(row) {
  await ElMessageBox.confirm(`确认删除通知模板 ${row.name}？`, '删除确认', { type: 'warning' })
  await deleteNotificationTemplate(row.id)
  ElMessage.success('已删除')
  await loadTemplates()
}

function templateTypeMeta(type) {
  return templateTypes.find(item => item.value === type) || { value: type, label: type || '-', icon: '' }
}

function formatTime(value) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}
</script>

<style scoped>
.task-page { --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 260px 120px auto; gap: 8px; }
.filter-bar > * { min-width: 0; width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.page-header-actions :deep(.el-button + .el-button), .row-actions :deep(.el-button + .el-button) { margin-left: 0; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }

.row-actions,
.type-cell {
  display: flex;
  align-items: center;
}

.row-actions {
  justify-content: center;
  gap: 4px;
}

.type-cell {
  justify-content: center;
  width: 46px;
  height: 30px;
  line-height: 0;
  margin: 0 auto;
}

.type-cell img {
  display: block;
  max-width: 34px;
  max-height: 26px;
  object-fit: contain;
}

:deep(.template-table.el-table) {
  --el-table-border-color: #edf0f5;
  --el-table-header-bg-color: #f7f9fc;
}

:deep(.template-table .el-table__header-wrapper th) {
  color: #5d6675;
  font-weight: 700;
}

:deep(.template-table .el-table__cell) {
  padding: 11px 0;
}

:deep(.template-table .el-table__row:hover > td.el-table__cell) {
  background: #f8fbff;
}

.row-actions :deep(.el-button) {
  width: 26px;
  height: 26px;
  margin-left: 0;
}

:deep(.template-drawer.el-drawer) {
  border-radius: 0;
}




.drawer-header {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  width: 100%;
}

.back-button {
  height: 32px;
  padding: 0 8px;
  font-size: 14px;
  color: var(--ops-primary);
}

.drawer-title {
  color: var(--ops-text);
  font-size: 16px;
  font-weight: 600;
}

.drawer-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.drawer-form :deep(.el-form-item__label) {
  margin-bottom: 6px;
  color: #4b5563;
  font-weight: 600;
  line-height: 20px;
}

.form-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.type-card-list {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  width: 100%;
}

.type-card {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100px;
  height: 100px;
  padding: 0;
  cursor: pointer;
  background: var(--ops-surface);
  border: 1px solid #d9d9d9;
  border-radius: 6px;
  outline: 1px solid transparent;
  outline-offset: -1px;
  transition: border-color .16s ease, box-shadow .16s ease;
}

.type-card.active {
  border-color: #1890ff;
  outline-color: #1890ff;
}

.type-card:hover {
  border-color: var(--ops-primary);
  box-shadow: 0 2px 8px rgba(31, 45, 61, .08);
}

.type-card img {
  display: block;
  max-width: 76px;
  max-height: 42px;
  width: auto;
  height: auto;
  margin: 0 auto;
  object-fit: contain;
}

.type-card .type-option > span {
  display: block;
  margin-top: 5px;
  color: var(--ops-text);
  font-size: 12px;
  line-height: 18px;
  text-align: center;
}

.type-card.active .type-option > span {
  color: #1890ff;
}

.type-option {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.template-notice {
  display: flex;
  min-height: 36px;
  box-sizing: border-box;
  align-items: center;
  gap: 9px;
  margin-bottom: 16px;
  padding: 7px 12px;
  color: var(--ops-muted);
  background: var(--ops-primary-soft);
  font-size: 11px;
}

.intro-icon {
  flex: none;
  color: var(--ops-primary);
  font-size: 14px;
}

.intro-message {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  color: var(--ops-muted);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.intro-message strong { color: var(--ops-muted); }

.template-fields {
  margin-bottom: 16px;
  padding: 12px 14px;
  background: #fbfcfe;
  border: 1px solid #e5e9f0;
  border-radius: 6px;
}

.template-fields summary {
  cursor: pointer;
  color: #303846;
  font-size: 13px;
  font-weight: 600;
}

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 16px;
  margin-top: 12px;
}

.field-item {
  display: grid;
  min-width: 0;
  grid-template-columns: minmax(150px, auto) 1fr;
  align-items: center;
  gap: 10px;
  font-size: 12px;
}

.field-item code {
  overflow: hidden;
  color: var(--ops-primary);
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field-item span { color: #7a8595; }

.color-legend {
  display: flex;
  flex-shrink: 0;
  gap: 6px;
}

.color-legend span {
  padding: 2px 7px;
  color: #fff;
  border-radius: 3px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
}

.color-legend .p0 { background: #d54941; }
.color-legend .p1 { background: #d97904; }
.color-legend .p2 { background: #3370ff; }
.color-legend .recovered { background: #2ea043; }

.help-icon {
  color: #1890ff;
}

.template-field-label {
  display: inline-flex;
  align-items: center;
  gap: 5px;
}

.template-editor :deep(textarea) {
  height: 500px;
  resize: none;
  font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  font-size: 13px;
  line-height: 1.5;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  width: 100%;
}

@media (max-width: 900px) {
  .task-toolbar { align-items: stretch; flex-wrap: wrap; }
  .filter-bar { width: 100%; flex: 1 1 100%; }
  .page-header-actions { margin-left: auto; }
  .task-pagination { overflow-x: auto; justify-content: flex-start; }
  .form-row {
    grid-template-columns: 1fr;
  }
  .template-notice {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .intro-message { overflow: visible; white-space: normal; }
  .color-legend { width: 100%; padding-left: 23px; }
  .field-grid { grid-template-columns: 1fr; }
}

@media (max-width: 560px) { .task-toolbar { flex-direction: column; }.filter-bar { grid-template-columns: 1fr 1fr; }.task-search { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; } }

.form-row,.field-grid { grid-template-columns:minmax(0,1fr); }
.template-notice { align-items:flex-start; flex-wrap:wrap; }
.intro-message { overflow:visible; white-space:normal; }
.color-legend { width:100%; padding-left:23px; }
</style>

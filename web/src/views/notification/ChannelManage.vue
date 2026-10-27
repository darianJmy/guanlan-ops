<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="通知对象筛选">
        <el-input v-model="keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索通知对象" placeholder="搜索名称、模板或备注" @input="handleFilterChange" @clear="applyFilters" />
        <el-select v-model="typeFilter" clearable aria-label="按通知类型筛选" placeholder="通知类型" @change="applyFilters">
          <el-option v-for="item in notificationTypes" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
        <el-select v-model="statusFilter" clearable aria-label="按状态筛选" placeholder="状态" @change="applyFilters"><el-option label="启用" :value="1" /><el-option label="停用" :value="0" /></el-select>
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadChannels">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openDrawer()" v-permission="['notification:channel:edit']">
          创建
        </el-button>
      </div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="pagedChannels" v-loading="loading" row-key="id" stripe class="channel-table">
        <el-table-column prop="name" label="通知对象" min-width="180" show-overflow-tooltip />
        <el-table-column label="通知类型" width="100" align="center">
          <template #default="{ row }">
            <span class="type-cell" :title="channelTypeText(row.type)">
              <img v-if="notificationTypeMeta(row.type).icon" :src="notificationTypeMeta(row.type).icon" :alt="channelTypeText(row.type)" />
              <span v-else>{{ channelTypeText(row.type) }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="通知模板" min-width="160" align="center" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.template_id ? templateName(row.template_id) : '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="备注" min-width="180" show-overflow-tooltip />
        <el-table-column label="更新时间" width="170">
          <template #default="{ row }">{{ formatTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="112" fixed="right" align="center">
          <template #default="{ row }">
            <div class="row-actions">
              <el-tooltip content="测试" placement="top"><el-button link type="primary" :icon="Promotion" :loading="testingId === row.id" aria-label="测试通知对象" @click="testOne(row)" /></el-tooltip>
              <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑通知对象" @click="openDrawer(row)" v-permission="['notification:channel:edit']" /></el-tooltip>
              <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除通知对象" @click="removeOne(row)" v-permission="['notification:channel:edit']" /></el-tooltip>
            </div>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div v-if="filteredChannels.length > 0" class="task-pagination"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="filteredChannels.length" :page-sizes="[10, 20, 50, 100]" layout="total, sizes, prev, pager, next" /></div>
    </el-card>

    <el-drawer v-model="drawerVisible" class="standard-config-drawer channel-drawer" size="min(460px, 94vw)" direction="rtl" :close-on-click-modal="false">
      <template #header>
        <div class="drawer-header">
          <DrawerBackButton @click="drawerVisible = false" />
          <span class="drawer-title">{{ form.id ? '编辑通知对象' : '创建通知对象' }}</span>
        </div>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="drawer-form">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="证书过期通知" />
        </el-form-item>
        <el-form-item label="通知类型" prop="type">
          <div class="type-card-list">
            <button
              v-for="item in notificationTypes"
              :key="item.value"
              class="type-card"
              :class="{ active: form.type === item.value, disabled: form.id && form.type !== item.value }"
              type="button"
              :disabled="!!form.id"
              @click="selectType(item.value)"
            >
              <span class="type-option">
                <img :src="item.icon" :alt="item.label" />
                <span>{{ item.label }}</span>
              </span>
            </button>
          </div>
        </el-form-item>
        <el-form-item v-if="form.type !== 'email'" prop="webhook_url">
          <template #label>
            <span class="form-label">
              Webhook 地址
              <el-tooltip :content="webhookHelpText" placement="top">
                <el-icon class="help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.webhook_url" :placeholder="webhookPlaceholder" @focus="selectInputText" />
        </el-form-item>
        <el-form-item v-if="['feishu', 'dingding'].includes(form.type)" prop="sign_secret">
          <template #label>
            <span class="form-label">
              签名密钥
              <el-tooltip :content="secretHelpText" placement="top">
                <el-icon class="help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-input
            v-model="form.sign_secret"
            type="password"
            show-password
            :placeholder="secretPlaceholder"
            @focus="selectInputText"
          />
        </el-form-item>
        <el-form-item v-if="form.type === 'email'" label="收件人邮箱" prop="email_recipients">
          <el-select v-model="form.email_recipients" multiple filterable allow-create default-first-option :reserve-keyword="false" placeholder="输入邮箱后按回车，可添加多个" />
        </el-form-item>
        <el-form-item v-if="!['webhook', 'customhook'].includes(form.type)" prop="template_id">
          <template #label>
            <span class="form-label">
              通知模板
              <el-tooltip content="只显示当前通知类型的模板，证书告警会优先使用通知对象绑定的模板渲染消息。" placement="top">
                <el-icon class="help-icon"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-select v-model="form.template_id" filterable placeholder="请选择通知模板">
            <el-option
              v-for="item in templateOptions"
              :key="item.id"
              :label="item.name"
              :value="item.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.description" type="textarea" :rows="3" />
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
import { ArrowLeft, Delete, Edit, Plus, Promotion, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import feishuIcon from '../../assets/feishu.svg'
import emailIcon from '../../assets/email.svg'
import dingdingIcon from '../../assets/dingding.svg'
import qywechatIcon from '../../assets/qywechat.svg'
import webhookIcon from '../../assets/customhook.svg'
import {
  createNotificationChannel,
  deleteNotificationChannel,
  getNotificationChannels,
  getNotificationTemplates,
  testNotificationChannel,
  updateNotificationChannel,
} from '../../api/notification'

const channels = ref([])
const templates = ref([])
const loading = ref(false)
const saving = ref(false)
const testingId = ref(null)
const drawerVisible = ref(false)
const formRef = ref()
const keyword = ref('')
const typeFilter = ref('')
const statusFilter = ref('')
const page = ref(1)
const pageSize = ref(10)
let filterTimer

const notificationTypes = [
  {
    value: 'feishu',
    label: '飞书',
    icon: feishuIcon,
    webhookPlaceholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...',
    webhookHint: '飞书机器人 Webhook 地址',
    secretPlaceholder: '启用签名校验时填写',
    secretHint: '机器人启用签名校验时，填写对应签名密钥；未启用可留空',
  },
  {
    value: 'dingding',
    label: '钉钉',
    icon: dingdingIcon,
    webhookPlaceholder: 'https://oapi.dingtalk.com/robot/send?access_token=...',
    webhookHint: '钉钉机器人 Webhook 地址',
    secretPlaceholder: '启用加签时填写 SEC 开头的密钥',
    secretHint: '机器人选择加签时需填写密钥；使用关键词或 IP 白名单时可留空，并确保消息和服务器符合机器人安全设置',
  },
  {
    value: 'qywechat',
    label: '企业微信',
    icon: qywechatIcon,
    webhookPlaceholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...',
    webhookHint: '企业微信群机器人的完整 Webhook 地址（包含 key）',
  },
  {
    value: 'email',
    label: '邮件',
    icon: emailIcon,

  },
  {
    value: 'webhook',
    label: '自定义 Webhook',
    icon: webhookIcon,
    webhookPlaceholder: 'https://example.com/webhook',
    webhookHint: '向此地址发送 JSON POST 请求，直接发送结构化事件数据，不使用通知模板；HTTP 2xx 表示发送成功',
  },
]

const form = reactive({
  email_recipients: [],
  id: null,
  name: '',
  type: 'feishu',
  template_id: null,
  webhook_url: '',
  sign_secret: '',
  description: '',
})

const currentTypeMeta = computed(() => notificationTypeMeta(form.type))
const templateOptions = computed(() => templates.value.filter(item => item.type === form.type))
const hasFilters = computed(() => keyword.value.trim() !== '' || typeFilter.value !== '' || statusFilter.value !== '')
const filteredChannels = computed(() => {
  const term = keyword.value.trim().toLowerCase()
  return channels.value.filter(item => {
    const matchesKeyword = !term || [item.name, item.description, templateName(item.template_id)].some(value => String(value || '').toLowerCase().includes(term))
    return matchesKeyword && (!typeFilter.value || item.type === typeFilter.value) && (statusFilter.value === '' || item.status === statusFilter.value)
  })
})
const pagedChannels = computed(() => filteredChannels.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
const webhookPlaceholder = computed(() => {
  if (form.id) return '留空则不修改'
  return currentTypeMeta.value.webhookPlaceholder
})
const secretPlaceholder = computed(() => {
  if (form.id) return '清空则删除签名密钥'
  return currentTypeMeta.value.secretPlaceholder
})
const webhookHelpText = computed(() => {
  if (form.id) return `${currentTypeMeta.value.webhookHint}。编辑时留空则保留原 Hook；填写新地址则覆盖。`
  return currentTypeMeta.value.webhookHint
})
const secretHelpText = computed(() => {
  if (form.id) return '编辑时清空并保存会删除签名密钥；填写内容则覆盖原签名密钥。'
  return currentTypeMeta.value.secretHint
})

const validateWebhook = (_rule, value, callback) => {
  if (form.type === 'email' || (form.id && !String(value || '').trim())) {
    callback()
    return
  }
  if (String(value || '').trim()) {
    callback()
    return
  }
  callback(new Error('请输入 Webhook 地址'))
}

const validateSignSecret = (_rule, value, callback) => {
  callback()
}

const rules = {
  email_recipients: [{ validator: (_r, value, done) => Array.isArray(value) && value.length && value.every(address => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(address)) ? done() : done(new Error('请添加有效的收件人邮箱')), trigger: 'change' }],
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  webhook_url: [{ required: true, validator: validateWebhook, trigger: 'blur' }],
  sign_secret: [{ validator: validateSignSecret, trigger: 'blur' }],
  template_id: [{ validator: (_rule, value, done) => ['webhook', 'customhook'].includes(form.type) || value ? done() : done(new Error('请选择通知模板')), trigger: 'change' }],
}

onMounted(() => {
  loadChannels()
  loadTemplates()
})
onBeforeUnmount(() => clearTimeout(filterTimer))

function handleFilterChange() { clearTimeout(filterTimer); filterTimer = setTimeout(applyFilters, 300) }
function applyFilters() { page.value = 1 }
function clearFilters() { keyword.value = ''; typeFilter.value = ''; statusFilter.value = ''; applyFilters() }

async function loadChannels() {
  loading.value = true
  try {
    const res = await getNotificationChannels()
    channels.value = res.data || []
    page.value = Math.min(page.value, Math.max(1, Math.ceil(filteredChannels.value.length / pageSize.value)))
  } finally {
    loading.value = false
  }
}

async function loadTemplates() {
  const res = await getNotificationTemplates()
  templates.value = res.data || []
}

function openDrawer(row) {
  Object.assign(form, row ? {
    email_recipients: [...(row.email_recipients || [])],
    id: row.id,
    name: row.name,
    type: row.type || 'feishu',
    template_id: row.template_id || null,
    webhook_url: row.webhook_url || '',
    sign_secret: row.sign_secret || '',
    description: row.description || '',
  } : {
    email_recipients: [],
    id: null,
    name: '',
    type: 'feishu',
    template_id: null,
    webhook_url: '',
    sign_secret: '',
    description: '',
  })
  drawerVisible.value = true
}

function selectType(type) {
  if (form.id) return
  if (form.type === type) return
  form.type = type
  form.template_id = null
  formRef.value?.clearValidate()
}

async function save() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = {
      name: form.name,
      type: form.type,
      template_id: ['webhook', 'customhook'].includes(form.type) ? 0 : form.template_id,
      webhook_url: form.type === 'email' ? '' : normalizedWebhookURL(),
      sign_secret: ['feishu', 'dingding'].includes(form.type) ? normalizedSignSecret() : '',
      email_recipients: form.type === 'email' ? form.email_recipients : undefined,
      status: 1,
      description: form.description,
    }
    if (form.id) await updateNotificationChannel(form.id, payload)
    else await createNotificationChannel(payload)
    ElMessage.success('通知对象已保存')
    drawerVisible.value = false
    await loadChannels()
  } finally {
    saving.value = false
  }
}

function normalizedWebhookURL() {
  if (!form.id) return form.webhook_url
  if (!String(form.webhook_url || '').trim()) return ''
  return form.webhook_url
}

function normalizedSignSecret() {
  if (!form.id) return form.sign_secret
  if (!String(form.sign_secret || '').trim()) return ''
  return form.sign_secret
}

function selectInputText(event) {
  event?.target?.select?.()
}

async function testOne(row) {
  testingId.value = row.id
  try {
    await testNotificationChannel(row.id)
    ElMessage.success('发送成功')
  } finally {
    testingId.value = null
  }
}

async function removeOne(row) {
  await ElMessageBox.confirm(`确认删除通知对象 ${row.name}？`, '删除确认', { type: 'warning' })
  await deleteNotificationChannel(row.id)
  ElMessage.success('已删除')
  await loadChannels()
}

function channelTypeText(type) {
  return notificationTypeMeta(type).label
}

function templateName(id) {
  const item = templates.value.find(template => template.id === id)
  return item?.name || '模板不存在'
}

function notificationTypeMeta(type) {
  return notificationTypes.find(item => item.value === type) || {
    value: type,
    label: type || '-',
    icon: '',
    webhookPlaceholder: 'https://example.com/webhook',
    webhookHint: 'Webhook 地址',
    secretPlaceholder: '可选',
    secretHint: '可选签名密钥',
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
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 260px 120px 100px auto; gap: 8px; }
.filter-bar > * { min-width: 0; width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.page-header-actions :deep(.el-button + .el-button), .row-actions :deep(.el-button + .el-button) { margin-left: 0; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }

.drawer-provider-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  border-radius: 6px;
  background: #f5f8fc;
  border: 1px solid #e7edf5;
}

.drawer-provider-icon img {
  width: 24px;
  height: 24px;
  object-fit: contain;
}

.type-cell {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 30px;
  line-height: 0;
}

.type-cell img {
  display: block;
  max-width: 34px;
  max-height: 26px;
  object-fit: contain;
}

:deep(.channel-table.el-table) {
  --el-table-border-color: #edf0f5;
  --el-table-header-bg-color: #f7f9fc;
}

:deep(.channel-table .el-table__header-wrapper th) {
  color: #5d6675;
  font-weight: 700;
}

:deep(.channel-table .el-table__cell) {
  padding: 11px 0;
}

:deep(.channel-table .el-table__row:hover > td.el-table__cell) {
  background: #f8fbff;
}

.row-actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
}

.row-actions :deep(.el-button) {
  width: 26px;
  height: 26px;
  margin-left: 0;
}

:deep(.channel-table .el-tag) { border-radius: 0; }

:deep(.channel-drawer.el-drawer) {
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

.drawer-provider {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 18px;
  color: var(--ops-text);
  font-weight: 700;
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

.drawer-form :deep(.el-select) {
  width: 100%;
}

.type-card-list {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  gap: 10px;
  width: 100%;
}

.type-card {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  min-width: 0;
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

.type-card.disabled {
  cursor: not-allowed;
  opacity: .45;
}

.type-card:hover {
  border-color: var(--ops-primary);
  box-shadow: 0 2px 8px rgba(31, 45, 61, .08);
}

.type-card.disabled:hover {
  border-color: #d9d9d9;
  box-shadow: none;
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

.form-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.help-icon {
  color: #7f8998;
  cursor: help;
  font-size: 14px;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  width: 100%;
}

@media (max-width: 980px) { .task-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; }.task-pagination { overflow-x: auto; justify-content: flex-start; } }
@media (max-width: 620px) { .task-toolbar { flex-direction: column; }.filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }.task-search { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; } }
@media (max-width: 400px) { .type-card-list { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>

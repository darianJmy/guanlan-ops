<template>
  <div class="cloud-monitor-page">
    <template v-if="!props.embedded && viewMode === 'list'">
      <section class="page-intro" role="note" aria-label="云监控2.0简介">
        <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
        <span>集中维护阿里云云监控 2.0 Workspace。名称用于平台内展示，Workspace 填写阿里云中的真实标识；AccessKey 加密保存且不会在页面中回显。</span>
      </section>

      <header class="page-toolbar">
        <el-input
          v-model="searchInput"
          clearable
          aria-label="搜索云监控2.0"
          placeholder="仪表盘名称"
          class="workspace-search"
          @keyup.enter="search"
          @clear="search"
        >
          <template #append>
            <el-button aria-label="搜索" @click="search"><el-icon><Search /></el-icon></el-button>
          </template>
        </el-input>
        <div class="toolbar-actions">
          <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate" v-permission="['dashboard:create']">创建</el-button>
        </div>
      </header>

      <div v-if="loading || items.length > 0" v-loading="loading" class="project-grid">
        <article v-for="item in items" :key="item.id" class="project-card">
          <button type="button" class="project-open" :aria-label="`打开云监控仪表盘 ${item.name}`" @click="openWorkspace(item)">
            <span class="project-logo" aria-hidden="true"><img :src="aliyunCloudLogo" alt="" /></span>
            <span class="project-content">
              <strong>{{ item.name }}</strong>
              <span v-if="item.description?.trim()" class="project-description">{{ item.description }}</span>
            </span>
          </button>
          <footer class="project-card-footer">
            <div class="project-actions">
              <el-tooltip content="编辑" placement="top">
                <el-button link type="primary" size="small" aria-label="编辑" @click="openEdit(item)" v-permission="['dashboard:edit']">
                  <el-icon><Edit /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="删除" placement="top">
                <el-button link type="danger" size="small" aria-label="删除" @click="remove(item)" v-permission="['dashboard:delete']">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </footer>
        </article>
      </div>

      <PageEmpty v-if="!loading && items.length === 0" :description="keyword ? '没有匹配的云监控 Workspace' : '暂无云监控2.0仪表盘'" />
    </template>

    <template v-else-if="viewMode === 'preview'">
      <header class="preview-header">
        <el-button link type="primary" :icon="ArrowLeft" @click="backToList">返回</el-button>
        <DashboardPreviewActions :url="previewUrl" :loading="sessionLoading" @refresh="refreshSession" />
      </header>
      <div class="iframe-shell">
        <div v-if="sessionLoading || iframeLoading" class="iframe-state">
          <el-icon :size="30" class="is-loading"><Loading /></el-icon>
          <span>{{ sessionLoading ? '正在创建云监控免登录会话...' : '正在加载云监控2.0...' }}</span>
        </div>
        <iframe
          v-if="previewUrl"
          :src="previewUrl"
          :title="`云监控 ${currentWorkspace?.name}`"
          referrerpolicy="no-referrer"
          @load="iframeLoading = false"
        />
      </div>
    </template>

    <el-drawer v-model="drawerVisible" class="standard-config-drawer cloud-monitor-drawer" size="min(460px, 94vw)" direction="rtl" :show-close="false" :close-on-click-modal="false">
      <template #header="{ titleId }">
        <div class="drawer-header">
          <DrawerBackButton :disabled="saving" @click="backFromForm" />
          <span :id="titleId" class="drawer-accessible-title">{{ '云监控2.0' }}</span>
        </div>
      </template>
      <DashboardTypeIdentity :icon="aliyunCloudLogo" label="云监控2.0" />
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="workspace-form">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" maxlength="128" aria-label="名称" />
        </el-form-item>
        <el-form-item prop="workspace">
          <template #label>
            <span class="field-label">Workspace
              <el-tooltip content="阿里云云监控 2.0 中的真实 Workspace 标识，用于打开对应控制台页面">
                <el-icon class="field-help"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.workspace" maxlength="128" aria-label="Workspace" />
        </el-form-item>
        <el-form-item label="地域" prop="region">
          <el-select v-model="form.region" filterable aria-label="地域">
            <el-option v-for="item in regionOptions" :key="item.value" :label="`${item.label}（${item.value}）`" :value="item.value" />
          </el-select>
        </el-form-item>
        <el-form-item :prop="form.id ? '' : 'access_key_id'">
          <template #label>
            <span class="field-label">AccessKey ID
              <el-tooltip :content="form.id ? '留空表示继续使用已保存的 AccessKey ID' : '用于创建阿里云云监控临时访问票据'">
                <el-icon class="field-help"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.access_key_id" type="password" show-password autocomplete="new-password" aria-label="AccessKey ID" />
        </el-form-item>
        <el-form-item :prop="form.id ? '' : 'access_key_secret'">
          <template #label>
            <span class="field-label">AccessKey Secret
              <el-tooltip :content="form.id ? '留空表示继续使用已保存的 AccessKey Secret' : '密钥会加密保存，保存后不再回显'">
                <el-icon class="field-help"><QuestionFilled /></el-icon>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.access_key_secret" type="password" show-password autocomplete="new-password" aria-label="AccessKey Secret" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" show-word-limit aria-label="描述" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="drawer-footer">
          <el-button class="test-button" :loading="testing" @click="testConnection">连接测试</el-button>
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
const props = defineProps({ embedded: Boolean })
const emit = defineEmits(['changed', 'view-change', 'select-type'])
defineExpose({ create: openCreate, edit: openEdit, open: openWorkspace })
import DashboardPreviewActions from './DashboardPreviewActions.vue'
import DashboardTypeIdentity from './DashboardTypeIdentity.vue'
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import PageEmpty from '../../components/PageEmpty.vue'
import { onMounted, reactive, ref, watch } from 'vue'
import { ArrowLeft, Delete, Edit, InfoFilled, Loading, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import aliyunCloudLogo from '../../assets/aliyun_cloud.svg'
import {
  createCloudMonitorSession,
  createCloudMonitorWorkspace,
  deleteCloudMonitorWorkspace,
  getCloudMonitorWorkspaces,
  testCloudMonitorWorkspace,
  updateCloudMonitorWorkspace,
} from '../../api/dashboard'

const regionOptions = [
  { value: 'cn-hangzhou', label: '华东 1（杭州）' },
  { value: 'cn-shanghai', label: '华东 2（上海）' },
  { value: 'cn-beijing', label: '华北 2（北京）' },
  { value: 'cn-shenzhen', label: '华南 1（深圳）' },
  { value: 'ap-southeast-1', label: '新加坡' },
  { value: 'eu-central-1', label: '德国（法兰克福）' },
  { value: 'us-west-1', label: '美国（硅谷）' },
]

const items = ref([])
const loading = ref(false)
const searchInput = ref('')
const keyword = ref('')
const drawerVisible = ref(false)
const saving = ref(false)
const testing = ref(false)
const formRef = ref()
const viewMode = ref('list')
const currentWorkspace = ref(null)
const previewUrl = ref('')
const sessionLoading = ref(false)
const iframeLoading = ref(false)
const form = reactive({ id: null, name: '', workspace: '', region: 'cn-hangzhou', access_key_id: '', access_key_secret: '', description: '' })
const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  workspace: [{ required: true, message: '请输入 Workspace', trigger: 'blur' }],
  region: [{ required: true, message: '请选择地域', trigger: 'change' }],
  access_key_id: [{ required: true, message: '请输入 AccessKey ID', trigger: 'blur' }],
  access_key_secret: [{ required: true, message: '请输入 AccessKey Secret', trigger: 'blur' }],
}

onMounted(() => { if (!props.embedded) fetchData() })
watch(viewMode, mode => emit('view-change', mode !== 'list'))

function search() {
  keyword.value = searchInput.value.trim()
  fetchData()
}

async function fetchData() {
  if (props.embedded) { emit('changed'); return }
  loading.value = true
  try {
    const res = await getCloudMonitorWorkspaces(keyword.value ? { keyword: keyword.value } : undefined)
    items.value = res.data || []
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, { id: null, name: '', workspace: '', region: 'cn-hangzhou', access_key_id: '', access_key_secret: '', description: '' })
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

function openEdit(row) {
  Object.assign(form, { id: row.id, name: row.name, workspace: row.workspace, region: row.region, access_key_id: '', access_key_secret: '', description: row.description || '' })
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

async function save() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (form.id) {
      await updateCloudMonitorWorkspace(form.id, { ...form })
      ElMessage.success('Workspace 已更新')
    } else {
      await createCloudMonitorWorkspace({ ...form })
      ElMessage.success('Workspace 已创建')
    }
    drawerVisible.value = false
    await fetchData()
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  testing.value = true
  try {
    await testCloudMonitorWorkspace({ ...form })
    ElMessage.success('连接成功')
  } finally {
    testing.value = false
  }
}

async function remove(row) {
  try {
    await ElMessageBox.confirm(`确定删除云监控仪表盘「${row.name}」吗？`, '删除确认', { type: 'warning' })
  } catch {
    return
  }
  await deleteCloudMonitorWorkspace(row.id)
  ElMessage.success('删除成功')
  await fetchData()
}

async function openWorkspace(row) {
  currentWorkspace.value = row
  viewMode.value = 'preview'
  await refreshSession()
}

async function refreshSession() {
  if (!currentWorkspace.value) return
  sessionLoading.value = true
  iframeLoading.value = false
  try {
    const res = await createCloudMonitorSession(currentWorkspace.value.id)
    previewUrl.value = res.data?.url || ''
    iframeLoading.value = !!previewUrl.value
  } finally {
    sessionLoading.value = false
  }
}

function backToList() {
  viewMode.value = 'list'
  previewUrl.value = ''
  currentWorkspace.value = null
}

function backFromForm() {
  drawerVisible.value = false
  if (props.embedded && !form.id) emit('select-type')
}
</script>

<style scoped>
.drawer-accessible-title { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.cloud-monitor-page { display: flex; min-height: 100%; flex-direction: column; gap: 16px; }
.page-intro { display: flex; min-height: 36px; align-items: center; gap: 9px; padding: 7px 12px; color: var(--ops-muted); background: var(--ops-primary-soft); font-size: 11px; }
.intro-icon { flex: none; color: var(--ops-primary); font-size: 14px; }
.page-toolbar, .preview-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.workspace-search { width: 240px; }
.toolbar-actions, .preview-actions { display: flex; align-items: center; gap: 10px; }
.project-grid { display: grid; min-height: 120px; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.project-card { display: flex; min-width: 0; overflow: hidden; flex-direction: column; background: var(--ops-surface); border: 1px solid var(--ops-border-soft); border-radius: 10px; transition: border-color .2s ease, box-shadow .2s ease, transform .2s ease; }
.project-card:hover { border-color: var(--ops-primary); box-shadow: 0 4px 16px rgba(64, 158, 255, .12); transform: translateY(-2px); }
.project-open { display: flex; width: 100%; min-width: 0; flex: 1; align-items: center; gap: 14px; padding: 20px 20px 14px; color: inherit; text-align: left; cursor: pointer; background: transparent; border: 0; }
.project-open:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
.project-actions { display: flex; align-items: center; gap: 8px; }
.project-actions .el-button + .el-button { margin-left: 0; }
.project-logo { display: grid; width: 52px; height: 52px; flex: 0 0 52px; place-items: center; background: #fff3e8; border-radius: 12px; }
.project-logo img { width: 34px; max-height: 34px; }
.project-content { display: flex; min-width: 0; flex: 1; flex-direction: column; }
.project-content strong { overflow: hidden; color: var(--ops-text); font-size: 15px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.project-description { display: -webkit-box; margin-top: 4px; overflow: hidden; color: var(--ops-muted); font-size: 12px; line-height: 18px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.project-card-footer { display: flex; align-items: center; justify-content: flex-end; padding: 10px 20px; background: var(--ops-surface-soft); border-top: 1px solid #f5f5f5; }
.iframe-shell { position: relative; min-height: 620px; flex: 1; overflow: hidden; background: var(--ops-card); border: 1px solid var(--ops-border); }
.iframe-shell iframe { display: block; width: 100%; height: calc(100vh - 150px); min-height: 620px; border: 0; }
.iframe-state { display: flex; position: absolute; z-index: 2; inset: 0; align-items: center; justify-content: center; flex-direction: column; gap: 12px; color: var(--ops-muted); background: color-mix(in srgb, var(--ops-card) 92%, transparent); }
:global(.cloud-monitor-drawer.el-drawer) { border-radius: 0; }
.drawer-header { display: flex; width: 100%; align-items: center; gap: 12px; }
.drawer-back { margin-left: -8px; color: var(--ops-primary); }
.workspace-form :deep(.el-select) { width: 100%; }
.field-label { display: inline-flex; align-items: center; gap: 5px; }
.field-help { color: #a3abb8; cursor: help; }
.drawer-footer { display: flex; width: 100%; align-items: center; justify-content: flex-end; gap: 8px; }
.test-button { margin-right: auto; }
@media (max-width: 760px) {
  .page-toolbar, .preview-header { align-items: stretch; flex-direction: column; }
  .workspace-search { width: 100%; }
  .toolbar-actions, .preview-actions { flex-wrap: wrap; }
  .project-grid { grid-template-columns: 1fr; }
}

@media (prefers-reduced-motion: reduce) {
  .project-card { transition: none; }
  .project-card:hover { transform: none; }
}
</style>

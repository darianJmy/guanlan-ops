<template>
  <div class="sls-page">
    <template v-if="!props.embedded && viewMode === 'projects'">
      <section class="project-intro" role="note" aria-label="日志仪表盘简介">
        <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
        <span class="intro-message">集中维护阿里云 SLS 与腾讯云 CLS 日志入口，按 Project 或 Topic 浏览日志，通过临时免登录会话快速进入日志查询。</span>
      </section>

      <div class="page-toolbar project-toolbar">
        <div class="project-search-actions">
          <el-input
            v-model="projectSearchInput"
            clearable
            aria-label="搜索日志仪表盘"
            placeholder="仪表盘名称"
            class="project-search"
            @keyup.enter="searchProjects"
            @clear="searchProjects"
          >
            <template #append>
              <el-button aria-label="搜索" @click="searchProjects">
                <el-icon><Search /></el-icon>
              </el-button>
            </template>
          </el-input>
        </div>
        <div class="toolbar-actions">
          <el-button :icon="Refresh" :loading="projectLoading" @click="fetchProjects">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="openCreate">创建</el-button>
        </div>
      </div>

      <div v-if="projectLoading || filteredProjects.length > 0" v-loading="projectLoading" class="project-grid">
        <div
          v-for="(item, index) in filteredProjects"
          :key="item.id"
          class="project-card"
          :class="{ dragging: dragIndex === index, over: dragOverIndex === index, 'is-sort-disabled': !!projectKeyword }"
          :draggable="!projectKeyword"
          @dragstart="onProjectDragStart($event, index)"
          @dragover.prevent="onProjectDragOver(index)"
          @dragleave="onProjectDragLeave"
          @drop.prevent="onProjectDrop(index)"
          @dragend="onProjectDragEnd"
        >
          <button type="button" class="project-open" @click="openProject(item)">
            <span class="project-logo" :class="`is-${item.provider}`"><img :src="providerMeta(item.provider).icon" alt="" /></span>
            <span class="project-content">
              <strong>{{ item.name }}</strong>
              <span v-if="item.description?.trim()" class="project-description">{{ item.description }}</span>
            </span>
          </button>
          <div class="project-card-footer">
            <div class="project-actions">
              <el-tooltip content="编辑" placement="top">
                <el-button link type="primary" size="small" aria-label="编辑" @click="openEdit(item)" v-permission="['dashboard:edit']">
                  <el-icon><Edit /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="删除" placement="top">
                <el-button link type="danger" size="small" aria-label="删除" @click="removeProject(item)" v-permission="['dashboard:delete']">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </div>
        </div>
      </div>

      <PageEmpty v-if="!projectLoading && filteredProjects.length === 0" :description="projects.length ? '没有匹配的日志仪表盘' : '暂无日志仪表盘'" />
    </template>

    <template v-else-if="viewMode === 'logstores'">
      <div class="subpage-header">
        <el-button link type="primary" :icon="ArrowLeft" @click="backToProjects">返回</el-button>
        <div class="toolbar-actions">
          <el-input v-model="logstoreKeyword" clearable :placeholder="`搜索 ${resourceLabel}`" class="project-search" @input="page = 1" />
          <el-button :loading="logstoreLoading" @click="fetchLogstores">刷新</el-button>
        </div>
      </div>

      <el-card shadow="never" class="logstore-card">
        <el-table v-loading="logstoreLoading" :data="pagedLogstores" row-key="id" stripe>
          <el-table-column prop="name" :label="resourceLabel" min-width="320">
            <template #default="{ row }">
              <button type="button" class="logstore-name" @click="openLogstore(row)">{{ row.name }}</button>
            </template>
          </el-table-column>
          <el-table-column :label="groupLabel" min-width="220"><template #default="{ row }">{{ row.group || currentProject?.project || '-' }}</template></el-table-column>
          <el-table-column label="区域" width="180"><template #default>{{ currentProject?.region }}</template></el-table-column>
          <template #empty><TableEmpty /></template>
        </el-table>
        <div class="pagination">
          <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="filteredLogstores.length"
            :page-sizes="[20, 50, 100]"
            layout="total, prev, pager, next, sizes"
          />
        </div>
      </el-card>
    </template>

    <template v-else-if="viewMode === 'preview'">
      <div class="preview-header">
        <el-button link type="primary" :icon="ArrowLeft" @click="backToLogstores">返回</el-button>
        <DashboardPreviewActions :url="previewUrl" :loading="sessionLoading" @refresh="createSession" />
      </div>
      <div class="iframe-shell">
        <div v-if="sessionLoading || iframeLoading" class="iframe-state">
          <el-icon :size="30" class="is-loading"><Loading /></el-icon>
          <span>{{ sessionLoading ? `正在创建 ${providerMeta(currentProject?.provider).shortName} 临时会话...` : '正在加载日志页面...' }}</span>
        </div>
        <iframe v-if="previewUrl" :src="previewUrl" :title="`${providerMeta(currentProject?.provider).shortName} ${currentResource?.name || ''}`" referrerpolicy="no-referrer" @load="iframeLoading = false" />
      </div>
    </template>

    <el-drawer v-model="drawerVisible" class="standard-config-drawer sls-config-drawer" size="min(460px, 94vw)" direction="rtl" :show-close="false" :close-on-click-modal="false">
      <template #header="{ titleId }">
        <div class="drawer-header">
          <DrawerBackButton :disabled="saving" @click="backFromForm" />
          <span :id="titleId" class="drawer-accessible-title">{{ providerMeta(form.provider).label }}</span>
        </div>
      </template>

      <DashboardTypeIdentity :icon="providerMeta(form.provider).icon" :label="providerMeta(form.provider).label" wide />
      <el-form ref="formRef" :model="form" :rules="formRules" label-position="top" class="sls-config-form">
        <el-form-item v-if="!props.embedded" label="日志服务类型" prop="provider">
          <div class="provider-selector">
            <button
              v-for="option in providerOptions"
              :key="option.value"
              type="button"
              class="provider-option"
              :class="{ selected: form.provider === option.value }"
              :disabled="!!form.id"
              @click="changeProvider(option.value)"
            >
              <img :src="option.icon" alt="" />
              <span><strong>{{ option.label }}</strong><small>{{ option.description }}</small></span>
              <el-icon v-if="form.provider === option.value"><Check /></el-icon>
            </button>
          </div>
        </el-form-item>
        <el-form-item prop="name">
          <template #label>
            <span class="field-label">
              配置名称
            </span>
          </template>
          <el-input v-model="form.name" aria-label="配置名称" />
        </el-form-item>
        <el-form-item prop="region">
          <template #label>
            <span class="field-label">
              Region
              <el-tooltip :content="form.provider === 'tencent_cls' ? '选择或输入 CLS 所在地域，例如：ap-shanghai' : '选择或输入 SLS Project 所在地域，例如：cn-hangzhou'" placement="top">
                <button type="button" class="field-help-button" aria-label="Region 填写说明">
                  <el-icon><QuestionFilled /></el-icon>
                </button>
              </el-tooltip>
            </span>
          </template>
          <el-select v-model="form.region" filterable allow-create default-first-option aria-label="日志服务 Region">
            <el-option v-for="region in currentRegionOptions" :key="region" :label="region" :value="region" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.provider === 'aliyun_sls'" prop="project">
          <template #label>
            <span class="field-label">
              Project
              <el-tooltip content="填写阿里云日志服务中的 Project 名称" placement="top">
                <button type="button" class="field-help-button" aria-label="Project 填写说明">
                  <el-icon><QuestionFilled /></el-icon>
                </button>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.project" aria-label="SLS Project" />
        </el-form-item>
        <el-form-item v-else prop="role_arn">
          <template #label>
            <span class="field-label">
              RoleArn
              <el-tooltip content="填写具备 CLS 只读权限、且允许当前子用户扮演的 CAM 角色 ARN" placement="top">
                <button type="button" class="field-help-button" aria-label="RoleArn 填写说明"><el-icon><QuestionFilled /></el-icon></button>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.role_arn" aria-label="腾讯云 RoleArn" />
        </el-form-item>
        <el-form-item :prop="form.id ? '' : 'access_key_id'">
          <template #label>
            <span class="field-label">
              {{ credentialIDLabel }}
              <el-tooltip :content="form.id ? `凭证已保存，留空表示不修改 ${credentialIDLabel}` : `填写用于访问日志服务的 ${credentialIDLabel}`" placement="top">
                <button type="button" class="field-help-button" :aria-label="`${credentialIDLabel} 填写说明`">
                  <el-icon><QuestionFilled /></el-icon>
                </button>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.access_key_id" :aria-label="credentialIDLabel" type="password" show-password autocomplete="new-password" />
        </el-form-item>
        <el-form-item :prop="form.id ? '' : 'access_key_secret'">
          <template #label>
            <span class="field-label">
              {{ credentialSecretLabel }}
              <el-tooltip :content="form.id ? `凭证已保存，留空表示不修改 ${credentialSecretLabel}` : `填写与 ${credentialIDLabel} 对应的 ${credentialSecretLabel}`" placement="top">
                <button type="button" class="field-help-button" :aria-label="`${credentialSecretLabel} 填写说明`">
                  <el-icon><QuestionFilled /></el-icon>
                </button>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.access_key_secret" :aria-label="credentialSecretLabel" type="password" show-password autocomplete="new-password" />
        </el-form-item>
        <el-form-item>
          <template #label>
            <span class="field-label">
              描述
            </span>
          </template>
          <el-input v-model="form.description" aria-label="日志仪表盘描述" type="textarea" :rows="3" maxlength="500" show-word-limit />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-footer">
          <el-button class="test-button" :loading="testing" @click="testProject">连接测试</el-button>
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveProject">保存</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
const props = defineProps({ embedded: Boolean })
const emit = defineEmits(['changed', 'view-change', 'select-type'])
defineExpose({ create: openCreate, edit: openEdit, open: openProject })
import DashboardPreviewActions from './DashboardPreviewActions.vue'
import DashboardTypeIdentity from './DashboardTypeIdentity.vue'
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import PageEmpty from '../../components/PageEmpty.vue'
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ArrowLeft, Check, Delete, Edit, InfoFilled, Loading, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  createSLSDashboardProject,
  createSLSDashboardSession,
  deleteSLSDashboardProject,
  getSLSDashboardLogstores,
  getSLSDashboardProjects,
  reorderSLSDashboardProjects,
  testSLSDashboardProject,
  updateSLSDashboardProject,
} from '../../api/dashboard'
import aliyunSLSIcon from '../../assets/aliyun_sls.svg'
import tencentCLSIcon from '../../assets/tencent_cls.svg'

const viewMode = ref('projects')
const projectLoading = ref(false)
const logstoreLoading = ref(false)
const sessionLoading = ref(false)
const iframeLoading = ref(false)
const projects = ref([])
const logstores = ref([])
const currentProject = ref(null)
const currentResource = ref(null)
const previewUrl = ref('')
const projectSearchInput = ref('')
const projectKeyword = ref('')
const logstoreKeyword = ref('')
const page = ref(1)
const pageSize = ref(20)
const drawerVisible = ref(false)
const saving = ref(false)
const testing = ref(false)
const dragIndex = ref(-1)
const dragOverIndex = ref(-1)
const formRef = ref()
const form = reactive({ id: null, provider: 'aliyun_sls', name: '', project: '', region: '', role_arn: '', access_key_id: '', access_key_secret: '', description: '' })
const formRules = {
  provider: [{ required: true, message: '请选择日志服务类型', trigger: 'change' }],
  name: [{ required: true, message: '请输入配置名称', trigger: 'blur' }],
  project: [{ required: true, message: '请输入 Project', trigger: 'blur' }],
  role_arn: [{ required: true, message: '请输入 RoleArn', trigger: 'blur' }],
  region: [{ required: true, message: '请选择或输入 Region', trigger: 'change' }],
  access_key_id: [{ required: true, message: '请输入访问密钥 ID', trigger: 'blur' }],
  access_key_secret: [{ required: true, message: '请输入访问密钥 Secret', trigger: 'blur' }],
}
const aliyunRegionOptions = [
  'cn-hangzhou', 'cn-shanghai', 'cn-beijing', 'cn-shenzhen', 'cn-guangzhou', 'cn-qingdao',
  'cn-zhangjiakou', 'cn-huhehaote', 'cn-chengdu', 'cn-hongkong', 'ap-southeast-1',
  'ap-southeast-5', 'ap-northeast-1', 'us-west-1', 'us-east-1', 'eu-central-1',
]
const tencentRegionOptions = [
  'ap-guangzhou', 'ap-shanghai', 'ap-beijing', 'ap-chengdu', 'ap-chongqing', 'ap-nanjing',
  'ap-hongkong', 'ap-singapore', 'ap-bangkok', 'ap-tokyo', 'ap-seoul', 'ap-jakarta',
  'na-siliconvalley', 'na-ashburn', 'eu-frankfurt', 'sa-saopaulo',
]
const providerOptions = [
  { value: 'aliyun_sls', label: '阿里云 SLS', shortName: 'SLS', description: 'Project / Logstore', icon: aliyunSLSIcon },
  { value: 'tencent_cls', label: '腾讯云 CLS', shortName: 'CLS', description: 'Region / Topic', icon: tencentCLSIcon },
]

const filteredProjects = computed(() => {
  const keyword = projectKeyword.value.trim().toLowerCase()
  if (!keyword) return projects.value
  return projects.value.filter(item => String(item.name || '').toLowerCase().includes(keyword))
})
const filteredLogstores = computed(() => {
  const keyword = logstoreKeyword.value.trim().toLowerCase()
  if (!keyword) return logstores.value
  return logstores.value.filter(item => `${item.name} ${item.group || ''}`.toLowerCase().includes(keyword))
})
const pagedLogstores = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredLogstores.value.slice(start, start + pageSize.value)
})
const resourceLabel = computed(() => currentProject.value?.provider === 'tencent_cls' ? 'Topic' : 'Logstore')
const groupLabel = computed(() => currentProject.value?.provider === 'tencent_cls' ? '日志集' : 'Project')
const credentialIDLabel = computed(() => form.provider === 'tencent_cls' ? 'SecretId' : 'AccessKey ID')
const credentialSecretLabel = computed(() => form.provider === 'tencent_cls' ? 'SecretKey' : 'AccessKey Secret')
const currentRegionOptions = computed(() => form.provider === 'tencent_cls' ? tencentRegionOptions : aliyunRegionOptions)

onMounted(() => { if (!props.embedded) fetchProjects() })
watch(viewMode, mode => emit('view-change', mode !== 'projects'))

function searchProjects() {
  projectKeyword.value = projectSearchInput.value
}

function providerMeta(provider) {
  return providerOptions.find(item => item.value === provider) || providerOptions[0]
}

function changeProvider(provider) {
  if (form.id || form.provider === provider) return
  form.provider = provider
  form.project = ''
  form.role_arn = ''
  form.region = provider === 'tencent_cls' ? 'ap-shanghai' : 'cn-hangzhou'
  form.access_key_id = ''
  form.access_key_secret = ''
  formRef.value?.clearValidate()
}

function onProjectDragStart(event, index) {
  if (projectKeyword.value) {
    event.preventDefault()
    return
  }
  dragIndex.value = index
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', String(index))
}

function onProjectDragOver(index) {
  if (dragIndex.value >= 0 && dragIndex.value !== index) dragOverIndex.value = index
}

function onProjectDragLeave() {
  dragOverIndex.value = -1
}

async function onProjectDrop(index) {
  const from = dragIndex.value
  dragIndex.value = -1
  dragOverIndex.value = -1
  if (from < 0 || from === index || projectKeyword.value) return

  const previous = projects.value.map(item => ({ ...item }))
  const reordered = previous.map(item => ({ ...item }))
  const [moved] = reordered.splice(from, 1)
  reordered.splice(index, 0, moved)
  projects.value = reordered.map((item, sort) => ({ ...item, sort }))

  try {
    await reorderSLSDashboardProjects(projects.value.map(item => item.id))
    ElMessage.success('排序已保存')
  } catch {
    projects.value = previous
    await fetchProjects()
  }
}

function onProjectDragEnd() {
  dragIndex.value = -1
  dragOverIndex.value = -1
}

async function fetchProjects() {
  if (props.embedded) { emit('changed'); return }
  projectLoading.value = true
  try {
    const res = await getSLSDashboardProjects()
    projects.value = res.data || []
  } finally {
    projectLoading.value = false
  }
}

async function openProject(project) {
  currentProject.value = project
  logstoreKeyword.value = ''
  page.value = 1
  viewMode.value = 'logstores'
  await fetchLogstores()
}

function openCreate(provider = 'aliyun_sls') {
  if (!providerOptions.some(option => option.value === provider)) provider = 'aliyun_sls'
  Object.assign(form, { id: null, provider, name: '', project: '', region: provider === 'tencent_cls' ? 'ap-shanghai' : 'cn-hangzhou', role_arn: '', access_key_id: '', access_key_secret: '', description: '' })
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

function openEdit(project) {
  Object.assign(form, {
    id: project.id,
    provider: project.provider || 'aliyun_sls',
    name: project.name,
    project: project.project,
    region: project.region,
    role_arn: project.role_arn || '',
    access_key_id: '',
    access_key_secret: '',
    description: project.description || '',
  })
  formRef.value?.clearValidate()
  drawerVisible.value = true
}

async function saveProject() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = { ...form }
    if (form.id) {
      await updateSLSDashboardProject(form.id, payload)
      ElMessage.success('日志仪表盘已更新')
    } else {
      await createSLSDashboardProject(payload)
      ElMessage.success('日志仪表盘已创建')
    }
    drawerVisible.value = false
    await fetchProjects()
  } finally {
    saving.value = false
  }
}

async function testProject() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  testing.value = true
  try {
    await testSLSDashboardProject({ ...form })
    ElMessage.success('连接成功')
  } finally {
    testing.value = false
  }
}

async function removeProject(project) {
  try {
    await ElMessageBox.confirm(`确定删除日志仪表盘「${project.name}」吗？`, '删除确认', { type: 'warning' })
  } catch {
    return
  }
  await deleteSLSDashboardProject(project.id)
  ElMessage.success('删除成功')
  await fetchProjects()
}

async function fetchLogstores() {
  if (!currentProject.value) return
  logstoreLoading.value = true
  try {
    const res = await getSLSDashboardLogstores(currentProject.value.id)
    logstores.value = res.data?.resources || (res.data?.logstores || []).map(name => ({ id: name, name, group: currentProject.value?.project || '' }))
    page.value = 1
  } finally {
    logstoreLoading.value = false
  }
}

async function openLogstore(resource) {
  currentResource.value = resource
  previewUrl.value = ''
  viewMode.value = 'preview'
  await createSession()
}

async function createSession() {
  if (!currentProject.value || !currentResource.value) return
  sessionLoading.value = true
  iframeLoading.value = false
  try {
    const res = await createSLSDashboardSession(currentProject.value.id, currentResource.value.id)
    previewUrl.value = res.data?.url || ''
    iframeLoading.value = !!previewUrl.value
  } finally {
    sessionLoading.value = false
  }
}

function backToProjects() {
  viewMode.value = 'projects'
  logstores.value = []
  currentProject.value = null
  currentResource.value = null
}

function backToLogstores() {
  viewMode.value = 'logstores'
  previewUrl.value = ''
  currentResource.value = null
}

function backFromForm() {
  drawerVisible.value = false
  if (props.embedded && !form.id) emit('select-type')
}
</script>

<style scoped>
.drawer-accessible-title { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

.sls-page { display: flex; min-height: 100%; flex-direction: column; gap: 16px; }
.page-toolbar, .subpage-header, .preview-header { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.project-intro { display: flex; min-height: 36px; align-items: center; gap: 9px; padding: 7px 12px; color: var(--ops-muted); background: var(--ops-primary-soft); font-size: 11px; }
.intro-icon { flex: none; color: var(--ops-primary); font-size: 14px; }
.intro-message { min-width: 0; overflow: hidden; color: var(--ops-muted); text-overflow: ellipsis; white-space: nowrap; }
.project-toolbar { padding: 0; border: 0; background: transparent; }
.project-search-actions { display: flex; align-items: center; }
.toolbar-actions { display: flex; align-items: center; gap: 10px; }
.project-search { width: 240px; }
.project-grid { display: grid; min-height: 120px; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.project-card { display: flex; min-width: 0; overflow: hidden; flex-direction: column; background: var(--ops-surface); border: 1px solid var(--ops-border-soft); border-radius: 10px; cursor: grab; transition: border-color .2s ease, box-shadow .2s ease, transform .2s ease; }
.project-card:active { cursor: grabbing; }
.project-card:hover { border-color: var(--ops-primary); box-shadow: 0 4px 16px rgba(64, 158, 255, .12); transform: translateY(-2px); }
.project-card.dragging { opacity: .4; transform: scale(.95); }
.project-card.over { background: var(--ops-primary-soft); border-color: var(--ops-primary); border-style: dashed; }
.project-card.is-sort-disabled { cursor: default; }
.project-open { display: flex; width: 100%; min-width: 0; flex: 1; align-items: center; gap: 14px; padding: 20px 20px 14px; color: inherit; text-align: left; cursor: inherit; background: transparent; border: 0; }
.project-open:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
.project-actions { display: flex; align-items: center; gap: 8px; }
.project-actions .el-button + .el-button { margin-left: 0; }
.project-logo { display: grid; width: 52px; height: 52px; flex: 0 0 52px; place-items: center; background: #fff3e8; border-radius: 12px; }
.project-logo.is-tencent_cls { background: #edf5ff; }
.project-logo img { width: 30px; max-height: 30px; }
.project-content { display: flex; min-width: 0; flex: 1; flex-direction: column; }
.project-content strong { overflow: hidden; color: var(--ops-text); font-size: 15px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.project-description { display: -webkit-box; margin-top: 4px; overflow: hidden; color: var(--ops-muted); font-size: 12px; line-height: 18px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.project-card-footer { display: flex; align-items: center; justify-content: flex-end; padding: 10px 20px; background: var(--ops-surface-soft); border-top: 1px solid #f5f5f5; }
.logstore-card { border-color: var(--ops-border); }
.logstore-name { padding: 0; color: var(--ops-primary); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-weight: 650; text-align: left; cursor: pointer; background: transparent; border: 0; }
.logstore-name:hover { text-decoration: underline; }
.logstore-name:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 3px; }
.pagination { display: flex; justify-content: flex-end; padding-top: 16px; }
.iframe-shell { position: relative; min-height: 620px; flex: 1; overflow: hidden; background: var(--ops-surface); border: 1px solid var(--ops-border); border-radius: 8px; }
.iframe-shell iframe { display: block; width: 100%; height: calc(100vh - 150px); min-height: 620px; border: 0; }
.iframe-state { display: flex; position: absolute; z-index: 2; inset: 0; align-items: center; justify-content: center; flex-direction: column; gap: 12px; color: var(--ops-muted); background: rgba(255, 255, 255, .92); }
:global(.sls-config-drawer.el-drawer) { border-radius: 0; }
.drawer-header { display: flex; width: 100%; align-items: center; gap: 12px; }
.drawer-back { margin-left: -8px; color: var(--ops-primary); }
.sls-config-form :deep(.el-select) { width: 100%; }
.provider-selector { display: grid; width: 100%; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.provider-option { display: grid; position: relative; min-width: 0; grid-template-columns: 38px minmax(0, 1fr) 18px; align-items: center; gap: 10px; padding: 13px 12px; border: 1px solid #dfe4eb; border-radius: 8px; color: #344054; background: var(--ops-surface); text-align: left; cursor: pointer; transition: border-color .2s, box-shadow .2s, background-color .2s; }
.provider-option:hover:not(:disabled) { border-color: #9ac7f5; }
.provider-option.selected { border-color: var(--ops-primary); background: #f3f8ff; box-shadow: 0 0 0 2px rgba(64, 158, 255, .1); }
.provider-option:disabled { cursor: default; opacity: .78; }
.provider-option img { width: 34px; max-height: 30px; object-fit: contain; }
.provider-option > span { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.provider-option strong { overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.provider-option small { color: var(--ops-subtle); font-size: 10px; }
.provider-option > .el-icon { color: var(--ops-primary); }
.field-label { display: inline-flex; align-items: center; gap: 5px; }
.field-help-button {
  display: inline-flex;
  width: 16px;
  height: 16px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  color: #a3abb8;
  background: transparent;
  cursor: help;
  font-size: 14px;
}
.field-help-button:hover,
.field-help-button:focus-visible { color: var(--ops-primary); }
.field-help-button:focus-visible {
  border-radius: 50%;
  outline: 2px solid rgba(64, 158, 255, .3);
  outline-offset: 1px;
}
.drawer-footer { display: flex; width: 100%; align-items: center; justify-content: flex-end; gap: 8px; }
.test-button { margin-right: auto; }
@media (max-width: 900px) {
  .page-toolbar, .subpage-header, .preview-header { align-items: stretch; flex-direction: column; }
  .project-search-actions { width: 100%; }
  .project-search-actions .project-search { width: auto; min-width: 0; flex: 1; }
  .toolbar-actions { flex-wrap: wrap; }
  .project-search { width: 100%; }
  .project-grid { grid-template-columns: 1fr; }
  .provider-selector { grid-template-columns: 1fr; }
}
</style>

<template>
  <div class="page-container">
    <!-- 列表视图 -->
    <template v-if="!props.embedded && viewMode === 'list'">
      <section class="dashboard-intro" role="note" aria-label="监控仪表盘简介">
        <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
        <span class="intro-message">集中查看基础设施、应用与业务监控。拖动卡片可调整顺序。</span>
      </section>

      <div class="dashboard-toolbar">
        <el-input
          v-model="dashboardSearchInput"
          clearable
          aria-label="搜索监控仪表盘"
          placeholder="仪表盘名称"
          class="dashboard-search"
          @keyup.enter="searchDashboards"
          @clear="searchDashboards"
        >
          <template #append>
            <el-button aria-label="搜索" @click="searchDashboards">
              <el-icon><Search /></el-icon>
            </el-button>
          </template>
        </el-input>
        <div class="toolbar-actions">
          <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="handleAdd" v-permission="['dashboard:create']">创建</el-button>
        </div>
      </div>

      <div v-if="loading || filteredDashboards.length > 0" v-loading="loading" class="dashboard-grid">
        <div
          v-for="(item, idx) in filteredDashboards"
          :key="item.id"
          class="dash-card"
          :class="{ dragging: dragIdx === idx, over: overIdx === idx }"
          :draggable="!dashboardKeyword"
          @click="enterDashboard(item)"
          @dragstart="onDragStart($event, idx)"
          @dragover.prevent="onDragOver(idx)"
          @dragleave="onDragLeave"
          @drop.prevent="onDrop(idx)"
          @dragend="onDragEnd"
        >
          <button type="button" class="dash-card-body" :aria-label="`查看仪表盘：${item.name}`" @click.stop="enterDashboard(item)">
            <span class="dash-card-brand">
              <span class="dash-card-icon">
                <img :src="dashboardConfig.icon" width="26" height="26" alt="" />
              </span>
            </span>
            <span class="dash-card-name" :title="item.name">{{ item.name }}</span>
            <span v-if="item.description?.trim()" class="dash-card-desc" :title="item.description">{{ item.description }}</span>
          </button>
          <div class="dash-card-footer">
            <div class="dash-card-actions" @click.stop>
              <el-tooltip content="编辑" placement="top">
                <el-button link type="primary" size="small" aria-label="编辑" @click="handleEdit(item)" v-permission="['dashboard:edit']">
                  <el-icon><Edit /></el-icon>
                </el-button>
              </el-tooltip>
              <el-tooltip content="删除" placement="top">
                <el-button link type="danger" size="small" aria-label="删除" @click="handleDelete(item)" v-permission="['dashboard:delete']">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </el-tooltip>
            </div>
          </div>
        </div>
      </div>

      <PageEmpty v-if="!loading && filteredDashboards.length === 0" :description="tableData.length ? '没有匹配的监控仪表盘' : '暂无监控仪表盘'" />
    </template>

    <!-- 预览视图 -->
    <template v-else-if="viewMode === 'preview'">
      <div class="preview-header">
        <el-button link type="primary" :icon="ArrowLeft" @click="viewMode = 'list'">返回</el-button>
        <DashboardPreviewActions :url="previewUrl" @refresh="refreshPreview" />
      </div>
      <div class="iframe-wrapper">
        <div v-if="iframeLoading" class="iframe-loading">
          <el-icon :size="32" class="is-loading"><Loading /></el-icon>
          <span>加载中...</span>
        </div>
        <iframe
          v-if="previewUrl"
          :key="previewKey"
          :src="previewUrl"
          class="preview-iframe"
          @load="iframeLoading = false"
        />
      </div>
    </template>

    <!-- 新增/编辑抽屉 -->
    <el-drawer v-model="drawerVisible" class="standard-config-drawer dashboard-drawer" size="min(460px, 94vw)" direction="rtl" :show-close="false" :close-on-click-modal="false" destroy-on-close>
      <template #header="{ titleId }">
        <div class="drawer-header">
          <DrawerBackButton :disabled="saving" @click="backFromForm" />
          <span :id="titleId" class="drawer-accessible-title">{{ props.provider === 'grafana' ? 'Grafana' : dashboardConfig.shortName }}</span>
        </div>
      </template>
      <DashboardTypeIdentity :icon="dashboardConfig.icon" :label="props.provider === 'grafana' ? 'Grafana' : dashboardConfig.shortName" />
      <el-form ref="formRef" class="dashboard-form" :model="form" :rules="rules" label-position="top">
        <el-form-item prop="name">
          <template #label>
            <span class="field-label">
              名称
            </span>
          </template>
          <el-input v-model="form.name" aria-label="仪表盘名称" />
        </el-form-item>
        <el-form-item prop="url">
          <template #label>
            <span class="field-label">
              URL
              <el-tooltip :content="`${dashboardConfig.urlPlaceholder}；${dashboardConfig.urlHelp}`" placement="top">
                <button type="button" class="field-help-button" aria-label="URL 填写说明">
                  <el-icon><QuestionFilled /></el-icon>
                </button>
              </el-tooltip>
            </span>
          </template>
          <el-input v-model="form.url" aria-label="仪表盘 URL" type="textarea" :rows="3" />
        </el-form-item>
        <el-form-item>
          <template #label>
            <span class="field-label">
              描述
            </span>
          </template>
          <el-input v-model="form.description" aria-label="仪表盘描述" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="drawer-footer">
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="handleSubmit">保存</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import DashboardPreviewActions from './DashboardPreviewActions.vue'
import DashboardTypeIdentity from './DashboardTypeIdentity.vue'
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import PageEmpty from '../../components/PageEmpty.vue'
import { computed, ref, reactive, onMounted, watch } from 'vue'
import { getDashboards, createDashboard, updateDashboard, deleteDashboard } from '../../api/dashboard'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Delete, Edit, InfoFilled, Loading, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import grafanaIcon from '../../assets/grafana_icon.svg'
import aliyunSLSIcon from '../../assets/aliyun_sls.svg'

const props = defineProps({ provider: { type: String, default: 'grafana' }, embedded: Boolean })
const emit = defineEmits(['changed', 'view-change', 'select-type'])
defineExpose({ create: handleAdd, edit: handleEdit, open: enterDashboard })
const dashboardConfigs = {
  grafana: {
    shortName: '监控仪表盘',
    icon: grafanaIcon,
    urlPlaceholder: 'Grafana iframe 链接，如 http://grafana:3000/d-solo/xxx?orgId=1&from=now-6h&to=now&panelId=1',
    urlHelp: '从 Grafana 面板 Share → Embed 获取 iframe 地址',
  },
  aliyun_sls: {
    shortName: '日志仪表盘',
    icon: aliyunSLSIcon,
    urlPlaceholder: '阿里云 SLS 仪表盘的免登录或已授权 iframe 地址',
    urlHelp: '请填写能够被浏览器 iframe 嵌入的 SLS 仪表盘地址，并确认跨域和登录授权配置',
  },
}
const dashboardConfig = computed(() => dashboardConfigs[props.provider] || dashboardConfigs.grafana)

const loading = ref(false)
const tableData = ref([])
const viewMode = ref('list')
const dashboardSearchInput = ref('')
const dashboardKeyword = ref('')

const filteredDashboards = computed(() => {
  const keyword = dashboardKeyword.value.trim().toLowerCase()
  if (!keyword) return tableData.value
  return tableData.value.filter(item => String(item.name || '').toLowerCase().includes(keyword))
})

const drawerVisible = ref(false)
const saving = ref(false)
const isEdit = ref(false)
const formRef = ref()
const form = reactive({ id: null, provider: props.provider, name: '', url: '', description: '', sort: 0, status: 1 })
const rules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  url: [{ required: true, message: '请输入 iframe URL', trigger: 'blur' }],
}

const previewUrl = ref('')
const iframeLoading = ref(true)
const previewKey = ref(0)

// ── 拖拽排序 ──
const dragIdx = ref(-1)
const overIdx = ref(-1)

function onDragStart(e, idx) {
  dragIdx.value = idx
  e.dataTransfer.effectAllowed = 'move'
  e.dataTransfer.setData('text/plain', String(idx))
}

function onDragOver(idx) {
  if (dragIdx.value !== idx) overIdx.value = idx
}

function onDragLeave() {
  overIdx.value = -1
}

async function onDrop(idx) {
  const from = dragIdx.value
  overIdx.value = -1
  dragIdx.value = -1
  if (from === idx || from < 0) return

  const previous = tableData.value.map(item => ({ ...item }))
  const list = previous.map(item => ({ ...item }))
  const [moved] = list.splice(from, 1)
  list.splice(idx, 0, moved)

  // 更新排序值
  list.forEach((item, i) => { item.sort = i })
  tableData.value = list

  // 持久化
  try {
    await Promise.all(list.map(item => updateDashboard(item.id, { sort: item.sort })))
    ElMessage.success('排序已保存')
  } catch {
    tableData.value = previous
    await fetchData()
  }
}

function onDragEnd() {
  dragIdx.value = -1
  overIdx.value = -1
}

// ── 业务 ──

onMounted(() => { if (!props.embedded) fetchData() })
watch(viewMode, mode => emit('view-change', mode !== 'list'))

function searchDashboards() {
  dashboardKeyword.value = dashboardSearchInput.value
}

async function fetchData() {
  if (props.embedded) { emit('changed'); return }
  loading.value = true
  try {
    const res = await getDashboards({ provider: props.provider })
    tableData.value = (res.data || []).sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0))
  } finally {
    loading.value = false
  }
}

function enterDashboard(row) {
  previewUrl.value = row.url
  iframeLoading.value = true
  viewMode.value = 'preview'
}

function handleAdd(nextSort) {
  isEdit.value = false
  const maxSort = tableData.value.reduce((m, i) => Math.max(m, i.sort ?? 0), 0)
  Object.assign(form, { id: null, provider: props.provider, name: '', url: '', description: '', sort: typeof nextSort === 'number' ? nextSort : maxSort + 1, status: 1 })
  drawerVisible.value = true
}

function handleEdit(row) {
  isEdit.value = true
  Object.assign(form, { id: row.id, provider: props.provider, name: row.name, url: row.url, description: row.description, sort: row.sort, status: row.status })
  drawerVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (isEdit.value) {
      await updateDashboard(form.id, { name: form.name, provider: form.provider, url: form.url, description: form.description, status: form.status })
      ElMessage.success('更新成功')
    } else {
      await createDashboard(form)
      ElMessage.success('创建成功')
    }
    drawerVisible.value = false
    await fetchData()
  } catch {}
  finally { saving.value = false }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除仪表盘「${row.name}」吗？`, '提示', { type: 'warning' })
  await deleteDashboard(row.id)
  ElMessage.success('删除成功')
  fetchData()
}

function refreshPreview() {
  if (!previewUrl.value) return
  iframeLoading.value = true
  previewKey.value++
}
function backFromForm() {
  drawerVisible.value = false
  if (props.embedded && !form.id) emit('select-type')
}
</script>

<style scoped>
.drawer-accessible-title { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }
.page-container { display: flex; flex-direction: column; gap: 16px; }
.dashboard-intro { display: flex; min-height: 36px; align-items: center; gap: 9px; padding: 7px 12px; color: var(--ops-muted); background: var(--ops-primary-soft); border-radius: 8px; font-size: 12px; }
.intro-icon { flex: none; color: var(--ops-primary); font-size: 14px; }
.intro-message { min-width: 0; color: var(--ops-muted); line-height: 1.6; }
.dashboard-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.dashboard-search { width: 200px; }
.toolbar-actions { display: flex; align-items: center; gap: 10px; }
.drawer-header { display: flex; width: 100%; align-items: center; gap: 12px; }
.drawer-back { margin-left: -8px; color: var(--ops-primary); }
.dashboard-form { padding: 0; }
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
  outline: 2px solid rgba(64, 158, 255, 0.3);
  outline-offset: 1px;
}
.drawer-footer { display: flex; justify-content: flex-end; gap: 8px; }
:global(.dashboard-drawer.el-drawer) { border-radius: 0 !important; }

/* ── 卡片网格 ── */
.dashboard-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 260px), 320px));
  align-items: stretch;
  gap: 16px;
}
.dash-card {
  border: 1px solid var(--ops-border-soft);
  min-width: 0;
  min-height: 218px;
  border-radius: 12px;
  background: var(--ops-surface);
  cursor: grab;
  transition: border-color .18s ease, box-shadow .18s ease, transform .18s ease;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.dash-card:active {
  cursor: grabbing;
}
.dash-card:hover,
.dash-card:focus-within {
  border-color: var(--ops-primary);
  box-shadow: var(--ops-shadow);
  transform: translateY(-2px);
}
.dash-card.dragging {
  opacity: 0.4;
  transform: scale(0.95);
}
.dash-card.over {
  border-color: var(--ops-primary);
  border-style: dashed;
  background: var(--ops-primary-soft);
}

.dash-card-body {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  width: 100%;
  min-width: 0;
  padding: 20px;
  border: 0;
  color: var(--ops-text);
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
.dash-card-body:focus-visible {
  outline: 2px solid var(--ops-primary);
  outline-offset: -3px;
  border-radius: 8px;
}
.dash-card-brand { display: flex; align-items: center; gap: 10px; margin-bottom: 4px; }
.dash-card-icon {
  display: grid;
  place-items: center;
  width: 42px;
  height: 42px;
  flex-shrink: 0;
  border: 1px solid var(--ops-border-soft);
  border-radius: 10px;
  background: var(--ops-surface-soft);
}
.dash-card-name {
  display: -webkit-box;
  max-width: 100%;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow-wrap: anywhere;
  color: var(--ops-text);
  font-size: 16px;
  font-weight: 650;
  line-height: 1.5;
}
.dash-card-desc {
  display: -webkit-box;
  max-width: 100%;
  overflow: hidden;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow-wrap: anywhere;
  color: var(--ops-muted);
  font-size: 12px;
  line-height: 1.7;
}
.dash-card-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  min-height: 48px;
  margin: 0 20px;
  padding: 10px 0;
  border-top: 1px solid var(--ops-border-soft);
}
.dash-card-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.dash-card-actions .el-button + .el-button { margin-left: 0; }

/* ── 预览视图 ── */
.preview-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.iframe-wrapper {
  position: relative;
  flex: 1;
  min-height: calc(100vh - 160px);
  border-radius: 8px;
  overflow: hidden;
  background: #f0f2f5;
}
.iframe-loading {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--ops-muted);
  font-size: 14px;
}
.preview-iframe {
  width: 100%;
  height: 100%;
  min-height: calc(100vh - 160px);
  border: none;
  position: relative;
  z-index: 1;
}
@media (max-width: 900px) {
  .dashboard-toolbar { align-items: stretch; flex-direction: column; }
  .dashboard-search { width: 100%; }
  .toolbar-actions { flex-wrap: wrap; }

}
@media (max-width: 480px) {
  .dashboard-grid { grid-template-columns: minmax(0, 1fr); }
  .dash-card { min-height: 200px; }
}
@media (prefers-reduced-motion: reduce) {
  .dash-card { transition: none; }
  .dash-card:hover, .dash-card:focus-within { transform: none; }
}
</style>

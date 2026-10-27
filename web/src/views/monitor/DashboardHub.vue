<template>
  <div class="dashboard-hub">
    <template v-if="!previewing">
      <section class="dashboard-intro" role="note" aria-label="仪表盘简介">
        <el-icon aria-hidden="true"><InfoFilled /></el-icon>
        <span>集中管理监控、日志与云监控仪表盘，点击卡片即可进入，直接拖动卡片可调整顺序。</span>
      </section>
      <div class="hub-toolbar">
        <div class="hub-filters">
          <el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索仪表盘名称或描述" aria-label="搜索仪表盘" />
        </div>
        <div class="hub-actions">
          <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="pickerVisible = true" v-permission="['dashboard:create']">创建</el-button>
        </div>
      </div>
      <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false" />
      <div v-loading="loading" class="hub-content">
        <div v-if="filteredItems.length" class="hub-grid">
          <article v-for="item in filteredItems" :key="dashboardKey(item)" class="hub-card" :draggable="canReorder" @dragstart="startDrag($event, item)" @dragend="endDrag" :class="{ 'is-dragging': dragging === dashboardKey(item), 'is-drop-target': dropTarget === dashboardKey(item) }" @click="openItem(item)" @dragover.prevent="overCard(item)" @dragleave.self="dropTarget = ''" @drop.prevent.stop="dropCard(item)">
            <button type="button" class="card-main" :draggable="canReorder" aria-keyshortcuts="Alt+ArrowLeft Alt+ArrowRight" @keydown.alt.left.prevent="moveItem(item, 'previous')" @keydown.alt.right.prevent="moveItem(item, 'next')" :aria-label="`打开${item.name}`" @click.stop="openItem(item)">
              <span class="card-brand"><img draggable="false" :src="iconFor(item)" :class="{ 'logo-wide': item.kind === 'logs', 'logo-cloud': item.kind === 'cloud' }" alt="" /><span>{{ labelFor(item) }}</span></span>
              <strong :title="item.name">{{ item.name }}</strong>
              <span v-if="item.description?.trim()" class="card-description" :title="item.description">{{ item.description }}</span>
            </button>
            <div v-if="canManage" class="card-footer" @click.stop @dragstart.stop.prevent>
              <div class="card-actions">
                <el-tooltip content="编辑" placement="top">
                  <el-button link type="primary" size="small" aria-label="编辑仪表盘" @click="editItem(item)" v-permission="['dashboard:edit']">
                    <el-icon><Edit /></el-icon>
                  </el-button>
                </el-tooltip>
                <el-tooltip content="删除" placement="top">
                  <el-button link type="danger" size="small" aria-label="删除仪表盘" @click="removeItem(item)" v-permission="['dashboard:delete']">
                    <el-icon><Delete /></el-icon>
                  </el-button>
                </el-tooltip>
              </div>
            </div>
          </article>
        </div>
        <PageEmpty v-else-if="!loading" :description="loadError ? '暂时无法显示仪表盘，请重试' : items.length ? '没有匹配的仪表盘' : '暂无仪表盘，点击创建选择接入类型'" />
      </div>
    </template>

    <component v-if="selectedType" :is="editors[selectedType]" ref="editor" embedded @changed="load" @view-change="previewing = $event" @select-type="pickerVisible = true" />

    <el-drawer v-model="pickerVisible" class="standard-config-drawer dashboard-type-drawer" size="min(320px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header="{ titleId, titleClass }">
        <div class="drawer-header">
          <DrawerBackButton @click="pickerVisible = false" />
          <span :id="titleId" :class="[titleClass, 'drawer-title']">选择仪表盘类型</span>
        </div>
      </template>
      <div class="type-groups">
        <section v-for="group in typeGroups" :key="group.name" class="type-group">
          <div class="type-group-title">{{ group.name }}</div>
          <div class="type-grid">
            <button v-for="type in group.items" :key="type.id" type="button" class="type-card" :aria-label="`创建${type.name}`" @click="createItem(type.id)">
              <span class="type-logo-slot"><img :src="type.icon" :class="{ 'logo-wide': type.kind === 'logs', 'logo-cloud': type.kind === 'cloud' }" alt="" /></span>
              <strong>{{ type.name }}</strong>
            </button>
          </div>
        </section>
      </div>
      <template #footer><el-button @click="pickerVisible = false">取消</el-button></template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, nextTick, onMounted, ref } from 'vue'
import { Delete, Edit, InfoFilled, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDashboardCards, reorderDashboardCards, deleteDashboard, deleteSLSDashboardProject, deleteCloudMonitorWorkspace } from '../../api/dashboard'
import { dashboardKey, moveDashboard } from '../../utils/dashboardOrder'
import { useUserStore } from '../../stores/user'
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import PageEmpty from '../../components/PageEmpty.vue'
import DashboardManage from './DashboardManage.vue'
import SLSDashboardManage from './SLSDashboardManage.vue'
import CloudMonitorDashboardManage from './CloudMonitorDashboardManage.vue'
import grafanaIcon from '../../assets/grafana_icon.svg'
import slsIcon from '../../assets/aliyun_sls.svg'
import clsIcon from '../../assets/tencent_cls.svg'
import cloudIcon from '../../assets/aliyun_cloud.svg'

const types = [
  { id: 'grafana', kind: 'grafana', name: 'Grafana', icon: grafanaIcon },
  { id: 'cloud', kind: 'cloud', name: '云监控2.0', icon: cloudIcon },
  { id: 'aliyun_sls', kind: 'logs', name: '阿里云 SLS', icon: slsIcon },
  { id: 'tencent_cls', kind: 'logs', name: '腾讯云 CLS', icon: clsIcon },
]
const typeGroups = [
  { name: '监控服务', items: types.filter(type => type.kind !== 'logs') },
  { name: '日志服务', items: types.filter(type => type.kind === 'logs') },
]
const editors = { grafana: DashboardManage, logs: SLSDashboardManage, cloud: CloudMonitorDashboardManage }
const remove = { grafana: deleteDashboard, logs: deleteSLSDashboardProject, cloud: deleteCloudMonitorWorkspace }
const store = useUserStore()
const canManage = computed(() => store.hasPermission('dashboard:edit') || store.hasPermission('dashboard:delete'))
const items = ref([]), keyword = ref(''), loading = ref(false), loadError = ref('')
const selectedType = ref(''), editor = ref(), pickerVisible = ref(false), previewing = ref(false)
const savingOrder = ref(false)
const dragging = ref(''), dropTarget = ref('')
let suppressOpenUntil = 0
const canReorder = computed(() => store.hasPermission('dashboard:edit') && !savingOrder.value && !loading.value && !loadError.value && !keyword.value.trim())
async function saveOrder(nextItems) {
  if (!canReorder.value) return
  const previous = items.value
  items.value = nextItems
  savingOrder.value = true
  try {
    await reorderDashboardCards(nextItems.map(({ kind, id }) => ({ kind, id })))
    await load()
  } catch {
    items.value = previous
    await load()
  } finally { savingOrder.value = false }
}
function moveItem(item, direction) {
  if (!canReorder.value) return
  const index = items.value.findIndex(row => dashboardKey(row) === dashboardKey(item))
  const target = direction === 'first' ? 0 : index + (direction === 'previous' ? -1 : 1)
  saveOrder(moveDashboard(items.value, dashboardKey(item), target))
}
function startDrag(event, item) {
  if (!canReorder.value) { event.preventDefault(); return }
  suppressOpenUntil = Infinity
  dragging.value = dashboardKey(item)
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('text/plain', dragging.value)
  event.dataTransfer.setDragImage(event.currentTarget.closest('.hub-card'), 24, 24)
}
function overCard(item) {
  if (canReorder.value && dragging.value) dropTarget.value = dashboardKey(item)
}
function dropCard(item) {
  if (canReorder.value && dragging.value) {
    const target = items.value.findIndex(row => dashboardKey(row) === dashboardKey(item))
    saveOrder(moveDashboard(items.value, dragging.value, target))
  }
  endDrag()
}
function endDrag() {
  suppressOpenUntil = Date.now() + 250
  dragging.value = ''
  dropTarget.value = ''
}
let requestID = 0
const filteredItems = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  return items.value.filter(item => `${item.name} ${item.description || ''}`.toLowerCase().includes(query))
})
function itemType(item) { return item.kind === 'logs' ? item.provider : item.kind }
function iconFor(item) { return types.find(type => type.id === itemType(item))?.icon }
function labelFor(item) { return types.find(type => type.id === itemType(item))?.name }
async function load() {
  const id = ++requestID
  loading.value = true
  try {
    const response = await getDashboardCards()
    if (id !== requestID) return
    items.value = response.data || []
    loadError.value = ''
  } catch {
    if (id === requestID) loadError.value = '仪表盘加载失败，请点击刷新重试'
  } finally { if (id === requestID) loading.value = false }
}
async function selectEditor(kind) {
  selectedType.value = kind
  await nextTick()
  return editor.value
}
async function createItem(typeID) {
  const type = types.find(type => type.id === typeID)
  if (!type) return
  const kind = type.kind
  pickerVisible.value = false
  const target = await selectEditor(kind)
  target.create(kind === 'logs' ? type.id : undefined)
}
async function editItem(item) { (await selectEditor(item.kind)).edit(item) }
async function openItem(item) {
  if (Date.now() < suppressOpenUntil) return
  try { await (await selectEditor(item.kind)).open(item) } catch { /* API interceptor displays the request error. */ }
}
async function removeItem(item) {
  try { await ElMessageBox.confirm(`确定删除仪表盘「${item.name}」吗？`, '删除仪表盘', { type: 'warning' }) } catch { return }
  await remove[item.kind](item.id)
  ElMessage.success('已删除仪表盘')
  await load()
}
onMounted(load)
</script>

<style scoped>
.dashboard-hub { display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.dashboard-intro { display: flex; align-items: center; gap: 9px; min-height: 36px; padding: 8px 12px; background: var(--ops-primary-soft); color: var(--ops-muted); font-size: 12px; line-height: 1.6; border-radius: 8px; }
.dashboard-intro .el-icon { flex: none; color: var(--ops-primary); }
.hub-toolbar,.hub-filters,.hub-actions { display: flex; align-items: center; gap: 12px; }
.hub-toolbar { justify-content: space-between; flex-wrap: wrap; }
.hub-filters { flex: 1; min-width: 0; }
.hub-filters .el-input { width: 260px; }
.hub-content { min-height: 180px; }
.hub-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 220px), 260px)); gap: 12px; }
.hub-card { position: relative; display: flex; min-width: 0; min-height: 170px; flex-direction: column; border: 1px solid var(--ops-border-soft); border-radius: 12px; background: var(--ops-surface); overflow: hidden; cursor: pointer; transition: border-color .18s, box-shadow .18s; }
.hub-card[draggable="true"], .card-main[draggable="true"] { cursor: grab; }
.hub-card[draggable="true"]:active, .card-main[draggable="true"]:active { cursor: grabbing; }
.hub-card.is-dragging { opacity: .45; }
.hub-card.is-drop-target { outline: 2px dashed var(--ops-primary); outline-offset: 2px; }
.hub-card:hover,.hub-card:focus-within { border-color: var(--ops-primary); box-shadow: var(--ops-shadow); }
.card-main { display: flex; flex: 1; flex-direction: column; align-items: flex-start; gap: 8px; width: 100%; padding: 14px; border: 0; color: var(--ops-text); background: transparent; text-align: left; font: inherit; cursor: pointer; }
.card-brand { display: flex; align-items: center; gap: 10px; color: var(--ops-muted); font-size: 12px; }
.card-brand img { width: 32px; height: 32px; object-fit: contain; }
.card-main strong,.card-description { display: -webkit-box; max-width: 100%; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; line-height: 1.6; }
.card-main strong { font-size: 14px; font-weight: 600; }
.card-description { font-size: 12px; color: var(--ops-muted); }
.card-footer { display: flex; align-items: center; justify-content: flex-end; padding: 10px 20px; background: var(--ops-surface-soft); border-top: 1px solid var(--ops-border-soft); }
.card-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.card-actions .el-button + .el-button { margin-left: 0; }
:global(.el-drawer.dashboard-type-drawer) { border-radius: 0; }
.drawer-header { display: flex; align-items: center; gap: 12px; }
.drawer-title { font-size: 16px; font-weight: 500; color: var(--ops-text); }
.type-groups { display: flex; flex-direction: column; gap: 24px; }
.type-group-title { margin-bottom: 10px; color: var(--ops-text); font-size: 14px; font-weight: 500; }
.type-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.type-card { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 0; min-width: 0; min-height: 96px; padding: 12px 6px; border: 1px solid var(--ops-border); border-radius: 8px; background: var(--ops-surface); color: var(--ops-text); text-align: center; font: inherit; cursor: pointer; }
.type-card:hover { border-color: var(--ops-primary); background: var(--ops-primary-soft); }
.type-logo-slot { display: flex; align-items: center; justify-content: center; width: 100%; height: 36px; flex-shrink: 0; }
.type-card img { display: block; width: 26px; height: 26px; object-fit: contain; }
.type-card img.logo-wide, .card-brand img.logo-wide { width: 88px; max-width: 100%; height: 32px; }
.type-card img.logo-cloud, .card-brand img.logo-cloud { width: 42px; height: 36px; }
.type-card strong { max-width: 100%; margin-top: 8px; overflow-wrap: anywhere; font-size: 12px; font-weight: 500; }
.type-card:focus-visible,.card-main:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
@media(max-width:600px) { .hub-filters { flex-basis: 100%; } .hub-filters .el-input { width: auto; flex: 1; } .hub-actions { margin-left: auto; } }
@media(max-width:480px) { .hub-grid { grid-template-columns: minmax(0,1fr); } }
@media(prefers-reduced-motion:reduce) { .hub-card { transition: none; } }
</style>

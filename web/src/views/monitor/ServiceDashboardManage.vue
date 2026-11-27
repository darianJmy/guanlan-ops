<template>
  <section class="service-dashboard">
    <section class="service-intro" role="note" aria-label="服务仪表盘简介">
      <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
      <span class="intro-message">常用服务统一入口，选择目录浏览服务，点击服务卡片即可在新窗口打开。</span>
    </section>

    <el-alert v-if="loadError" :title="loadError" type="error" show-icon :closable="false">
      <template #default>正在自动重试，请检查服务访问后端，也可点击刷新。</template>
    </el-alert>
    <div class="page-toolbar service-toolbar">
      <div class="service-filter-actions">
        <el-input v-model="keyword" :prefix-icon="Search" clearable placeholder="搜索服务名称、地址或 Service" aria-label="搜索服务" class="service-search" />
        <el-select v-model="clusterFilter" clearable placeholder="全部来源" aria-label="筛选服务来源" class="filter-select">
          <el-option v-for="item in clusterOptions" :key="item.id" :value="item.id" :label="item.name" />
        </el-select>
        <el-select v-model="statusFilter" clearable placeholder="全部状态" aria-label="筛选服务状态" class="filter-select">
          <el-option v-for="(value, key) in serviceStatuses" :key="key" :value="key" :label="value.label" />
        </el-select>
      </div>
      <div class="toolbar-actions">
        <span class="service-count">{{ childFolderCount }} 个目录 · {{ currentFolderResources.length }} 个服务<span v-if="currentFolderResources.length"> · {{ runningCount }} 个运行中</span></span>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
        <el-dropdown v-if="canCreate" trigger="click" @command="handleCreateCommand">
          <el-button type="primary" :icon="Plus" :disabled="!available">创建<el-icon class="create-caret"><ArrowDown /></el-icon></el-button>
          <template #dropdown><el-dropdown-menu><el-dropdown-item command="service" :icon="Plus">服务</el-dropdown-item><el-dropdown-item command="folder" :icon="FolderAdd" divided>目录</el-dropdown-item></el-dropdown-menu></template>
        </el-dropdown>
      </div>
    </div>

    <div class="dashboard-browser">
      <DashboardFolderTree ref="folderTreeRef" :model-value="activeFolderId" scope="services" @update:model-value="navigateFolder" @invalid-folder="resetFolderRoute" @folders-change="folders = $event" @permissions-change="refreshResources" @move-item="moveResourceToFolder">
        <template #folder-card="{ folder, open }">
          <button type="button" class="dashboard-entry-main folder-service-open" @click="open">
            <span class="service-icon dashboard-entry-icon"><el-icon><Folder /></el-icon></span>
            <span class="dashboard-entry-content">
              <strong class="dashboard-entry-name" :title="folder.name">{{ folder.name }}</strong>
              <span class="dashboard-entry-description">服务目录</span>
            </span>
            <el-icon class="folder-service-arrow"><ArrowRight /></el-icon>
          </button>
          <div class="service-card-content">
            <dl class="service-details">
              <div><dt>子目录</dt><dd>{{ folderSummary(folder).folders }} 个</dd></div>
              <div><dt>直属服务</dt><dd>{{ loading || loadError ? '—' : `${folderSummary(folder).services} 个` }}</dd></div>
              <div><dt>运行中</dt><dd>{{ loading || loadError ? '—' : `${folderSummary(folder).running} 个` }}</dd></div>
            </dl>
          </div>
          <footer class="dashboard-entry-footer service-card-footer">
            <el-button size="small" :icon="FolderOpened" @click="open">打开目录</el-button>
          </footer>
        </template>
      </DashboardFolderTree>
      <section class="dashboard-content">
        <div v-if="visibleResources.length" v-loading="loading" class="service-grid dashboard-entry-grid">
          <article
            v-for="item in visibleResources" :key="item.resource.id"
            class="service-card dashboard-entry-card"
            :class="{ 'is-draggable': canManageResource(item), 'is-openable': canOpenService(item), 'service-card--kubernetes': !isHTTPResource(item.resource), 'service-card--proxy': item.resource.type === 'http', 'service-card--direct': isDirectResource(item.resource) }"
            role="link" :tabindex="canOpenService(item) ? 0 : -1"
            :aria-label="`打开服务 ${item.resource.name}（新窗口）`" :aria-disabled="!canOpenService(item)"
            :aria-busy="opening === item.resource.id"
            :draggable="canManageResource(item)"
            @click="openResource(item)"
            @keydown.enter.self.prevent="openResource(item)"
            @keydown.space.self.prevent="openResource(item)"
            @dragstart="startResourceDrag($event, item)" @dragend="finishResourceDrag"
          >
            <div class="dashboard-entry-main">
              <span class="service-icon dashboard-entry-icon"><el-icon v-if="isDirectResource(item.resource)" aria-label="HTTP URL 直连"><TopRight /></el-icon><el-icon v-else-if="isHTTPResource(item.resource)" aria-label="HTTP URL 代理"><Connection /></el-icon><img v-else :src="kubernetesLogo" alt="Kubernetes" /></span>
              <div class="dashboard-entry-content">
                <strong class="dashboard-entry-name" :title="item.resource.name">{{ item.resource.name }}</strong>
                <span class="dashboard-entry-description">{{ serviceDescription(item) }}</span>
              </div>
              <el-tag size="small" :type="statusInfo(item).type">{{ statusInfo(item).label }}</el-tag>
            </div>
            <div class="service-card-content">
              <dl class="service-details">
                <template v-if="isHTTPResource(item.resource)">
                  <div><dt>目标地址</dt><dd :title="item.resource.targetURL">{{ item.resource.targetURL }}</dd></div>
                  <div v-if="!isDirectResource(item.resource)"><dt>访问端口</dt><dd>{{ item.resource.publicPort }}</dd></div>
                  <template v-else>
                    <div><dt>访问方式</dt><dd>浏览器直达</dd></div>
                    <div><dt>认证方式</dt><dd>目标服务认证</dd></div>
                  </template>
                </template>
                <template v-else>
                  <div><dt>命名空间</dt><dd :title="item.resource.namespace">{{ item.resource.namespace }}</dd></div>
                  <div><dt>Service</dt><dd :title="`${item.resource.service}:${item.resource.port}`">{{ item.resource.service }}:{{ item.resource.port }}</dd></div>
                  <div v-if="isTCPResource(item.resource)"><dt>访问端口</dt><dd>{{ item.resource.publicPort }}</dd></div>
                </template>
                <div v-if="!isDirectResource(item.resource)"><dt>打开路径</dt><dd :title="item.resource.path">{{ item.resource.path || '/' }}</dd></div>
              </dl>
              <p v-if="item.error || item.resource.lastError" class="resource-error" :title="item.error || item.resource.lastError">{{ item.error || item.resource.lastError }}</p>
            </div>
            <footer class="dashboard-entry-footer service-card-footer">
              <div v-if="canManageResource(item)" class="resource-actions" @click.stop @keydown.stop>
                <el-tooltip :content="canEditService(item) ? '编辑服务' : '停用并关闭连接后可编辑'">
                  <span><el-button link :icon="Edit" aria-label="编辑服务" :disabled="!available || !!busy.id || !canEditService(item)" @click="editResource(item)" /></span>
                </el-tooltip>
                <el-button link type="danger" :icon="Delete" aria-label="删除服务" :disabled="!available || !!busy.id" @click="removeResource(item)" />
                <el-button v-if="!isDirectResource(item.resource) && item.resource.desired" link :loading="busy.id === item.resource.id && busy.action === 'stop'" :disabled="!available || !!busy.id" @click="changeRunning(item, false)">停用</el-button>
                <el-button v-else-if="!isDirectResource(item.resource)" link type="primary" :loading="busy.id === item.resource.id && busy.action === 'start'" :disabled="!available || !!busy.id || serviceStatus(item) === 'stopping'" @click="changeRunning(item, true)">启用</el-button>
              </div>
              <el-button type="primary" size="small" :icon="TopRight" :loading="opening === item.resource.id" :disabled="!canOpenService(item)" @click.stop="openResource(item)">打开服务</el-button>
            </footer>
          </article>
        </div>
        <p v-if="!loading && !loadError && !visibleResources.length && childFolderCount && !hasFilters" class="folder-hint">选择上方目录查看服务</p>
        <el-empty v-else-if="!loading && !loadError && !visibleResources.length" :description="hasFilters ? '当前目录没有匹配的服务' : '当前目录暂无可访问的服务'">
          <el-button v-if="hasFilters" @click="clearFilters">清除筛选</el-button>
          <el-button v-else-if="canCreate" type="primary" :disabled="!available" @click="editResource()">添加服务</el-button>
        </el-empty>
      </section>
    </div>

    <el-drawer v-model="resourceVisible" :title="resourceForm.id ? '编辑服务' : '添加服务'" size="min(600px, 96vw)" :close-on-click-modal="false" @closed="resetDiscovery">
      <el-form ref="resourceFormRef" :model="resourceForm" :rules="resourceRules" label-position="top" @submit.prevent>
        <el-form-item label="服务名称" prop="name"><el-input v-model="resourceForm.name" maxlength="128" placeholder="例如：服务管理台" /></el-form-item>
        <el-form-item label="目录"><el-tree-select v-model="resourceForm.folder_id" :data="folderSelectTree" node-key="id" :props="{ label: 'name', children: 'children' }" check-strictly default-expand-all aria-label="服务目录" /></el-form-item>
        <el-form-item label="服务来源">
          <el-radio-group :model-value="isHTTPResource(resourceForm) ? 'url' : 'kubernetes'" aria-label="服务来源" @change="resourceSourceChanged">
            <el-radio-button value="kubernetes">Kubernetes Service</el-radio-button>
            <el-radio-button value="url">HTTP / HTTPS URL</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="访问方式" prop="type">
          <el-radio-group v-model="resourceForm.type" class="resource-access-options" aria-label="访问方式" @change="resourceTypeChanged">
            <el-radio v-for="option in resourceAccessOptions" :key="option.value" :value="option.value" border class="resource-access-option">
              <strong>{{ option.label }}</strong>
              <span class="resource-access-description">{{ option.description }}</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="isHTTPResource(resourceForm)" label="目标 URL" prop="targetURL"><el-input v-model="resourceForm.targetURL" maxlength="2048" placeholder="https://app.example.com/console/" /><span class="field-help">填写完整 HTTP 或 HTTPS 地址，可包含端口、路径和查询参数。</span></el-form-item>
        <template v-else>
          <el-alert v-if="datasourceLoadError" :title="datasourceLoadError" type="error" :closable="false" show-icon class="discovery-error" />
          <el-alert v-else-if="!loadingDatasources && !clusters.length" title="尚未配置 Kubernetes 接入" type="info" :closable="false" show-icon class="discovery-error"><template #default><el-button link type="primary" @click="goToKubernetesSources">前往接入中心配置</el-button></template></el-alert>
          <el-form-item label="Kubernetes 接入" prop="clusterId"><el-select v-model="resourceForm.clusterId" filterable placeholder="选择接入中心中的 Kubernetes 数据源" :disabled="discoveryLoading || loadingDatasources" :loading="loadingDatasources" @change="clusterChanged"><el-option v-for="cluster in clusters" :key="cluster.id" :value="cluster.id" :label="cluster.name" /></el-select><span class="field-help">集群凭据和连接设置统一在“接入中心 → Kubernetes”维护。</span></el-form-item>
          <el-alert v-if="discoveryError" :title="discoveryError" type="error" :closable="false" show-icon class="discovery-error"><template #default><el-button link type="primary" :loading="discoveryLoading" @click="retryDiscovery">重新获取</el-button></template></el-alert>
          <el-form-item label="命名空间" prop="namespace"><el-select v-model="resourceForm.namespace" filterable placeholder="选择命名空间" :disabled="!resourceForm.clusterId || discoveryLoading" :loading="discoveryLoading" @change="namespaceChanged"><el-option v-for="namespace in namespaceOptions" :key="namespace" :value="namespace" :label="namespace" /></el-select></el-form-item>
          <el-form-item label="Service" prop="service"><el-select v-model="resourceForm.service" filterable placeholder="选择 Service" :disabled="!resourceForm.namespace || discoveryLoading" @change="serviceChanged"><el-option v-for="service in serviceOptions" :key="service.name" :value="service.name" :label="service.name" /></el-select></el-form-item>
          <el-form-item label="服务端口" prop="port"><el-select v-model="resourceForm.port" :filterable="isExternalName" :allow-create="isExternalName" :default-first-option="isExternalName" :placeholder="isExternalName ? '选择或输入目标 TCP 端口' : '选择 TCP 端口'" :disabled="!resourceForm.service || discoveryLoading"><el-option v-for="port in portOptions" :key="port.value" :value="port.value" :label="port.label" /></el-select><span v-if="isExternalName" class="field-help">目标地址：{{ selectedService.externalName }}。请选择或输入目标应用的 TCP 端口（1–65535）。</span></el-form-item>
          <el-form-item label="打开路径" prop="path"><el-input v-model="resourceForm.path" placeholder="/ 或 /nacos/" maxlength="1000" /></el-form-item>
        </template>
        <el-form-item v-if="!isDirectResource(resourceForm)" label="连接模式" prop="onDemand">
          <el-radio-group v-model="resourceForm.onDemand" aria-label="连接模式">
            <el-radio-button :value="false">常驻连接</el-radio-button>
            <el-radio-button :value="true">按需连接</el-radio-button>
          </el-radio-group>
          <span v-if="resourceForm.onDemand" class="field-help">访问时建立连接，空闲约 10 分钟后释放。</span>
        </el-form-item>
        <el-form-item v-if="!isDirectResource(resourceForm)" label="访问端口" prop="publicPort"><el-input-number v-model="resourceForm.publicPort" :min="portalInfo.minPort" :max="portalInfo.maxPort" controls-position="right" /><span class="field-help">范围 {{ portalInfo.minPort }}–{{ portalInfo.maxPort }}，每个服务使用独立端口。</span></el-form-item>
        <el-alert v-if="isDirectResource(resourceForm) || (!resourceForm.id && !resourceForm.onDemand)" :title="isDirectResource(resourceForm) ? '保存后即可打开，无需启停或配置代理端口。' : '保存后自动建立访问连接，连接成功后即可打开。'" type="info" :closable="false" show-icon />
      </el-form>
      <template #footer><el-button :disabled="savingResource" @click="resourceVisible = false">取消</el-button><el-button type="primary" :loading="savingResource" @click="saveResource">保存</el-button></template>
    </el-drawer>

  </section>
</template>

<script setup>
import { computed, onActivated, onBeforeUnmount, onDeactivated, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, ArrowRight, Connection, Delete, Edit, Folder, FolderAdd, FolderOpened, InfoFilled, Plus, Refresh, Search, TopRight } from '@element-plus/icons-vue'
import { useUserStore } from '../../stores/user'
import kubernetesLogo from '../../assets/kubernetes.svg'
import { getDatasources } from '../../api/datasource'
import { createPortalAccess, deletePortalResource, getPortalNamespaces, getPortalResources, getPortalServices, getServicePortalInfo, movePortalResource, savePortalResource, setPortalResourceRunning } from '../../api/servicePortal'
import { canEditService, canManageServiceResource, httpTargetURL, portalAccessURL, serviceStatus, serviceStatuses } from '../../utils/serviceDashboard'
import { dashboardFolderSelectTree } from '../../utils/dashboardFolders'
import DashboardFolderTree from '../../components/DashboardFolderTree.vue'

defineOptions({ name: 'ServiceDashboardManage' })

const store = useUserStore()
const canManage = computed(() => store.hasPermission('service-dashboard:manage') && portalInfo.user?.admin === true)
const portalInfo = reactive({ minPort: 18000, maxPort: 18999, user: null })
const route = useRoute(), router = useRouter()
const folderIdFromRoute = () => Number.isInteger(Number(route.query.folder)) && Number(route.query.folder) > 0 ? Number(route.query.folder) : 0
const activeFolderId = ref(folderIdFromRoute()), folders = ref([]), folderTreeRef = ref()
const activeFolderAccess = computed(() => activeFolderId.value ? folders.value.find(folder => Number(folder.id) === activeFolderId.value)?.access_level || 'none' : 'manage')
const canCreate = computed(() => canManage.value && ['edit', 'manage'].includes(activeFolderAccess.value))
const folderSelectTree = computed(() => dashboardFolderSelectTree(folders.value.filter(folder => ['edit', 'manage'].includes(folder.access_level))))
const canManageResource = item => canManageServiceResource(item, canManage.value)
const resources = ref([]), datasources = ref([])
function folderSummary(folder) {
  const entries = resources.value.filter(item => Number(item.resource.folder_id || 0) === Number(folder.id))
  return {
    folders: folders.value.filter(item => Number(item.parent_id || 0) === Number(folder.id)).length,
    services: entries.length,
    running: entries.filter(item => serviceStatus(item) === 'running').length,
  }
}
const clusters = computed(() => datasources.value.filter(item => item.type === 'k8s' && ['client_certificate', 'bearer'].includes(String(item.auth_type || '').toLowerCase()) && Number(item.status) === 1 && !Number(item.tunnel_id)).map(item => ({ ...item, id: String(item.id), name: item.name })))
const loading = ref(false), available = ref(false), loadError = ref(''), datasourceLoadError = ref(''), loadingDatasources = ref(false)
const keyword = ref(''), clusterFilter = ref(''), statusFilter = ref('')
const hasFilters = computed(() => !!(keyword.value.trim() || clusterFilter.value || statusFilter.value))
const childFolderCount = computed(() => folders.value.filter(folder => Number(folder.parent_id || 0) === activeFolderId.value).length)
function clearFilters() {
  keyword.value = ''
  clusterFilter.value = ''
  statusFilter.value = ''
}
const opening = ref('')
const busy = reactive({ id: '', action: '' })
const isDirectResource = resource => resource.type === 'http_direct'
function canOpenService(item) {
  if (!available.value || opening.value || busy.id) return false
  if (isDirectResource(item.resource)) return true
  const status = serviceStatus(item)
  if (item.resource.onDemand) {
    return item.resource.desired && ['idle', 'connecting', 'failed', 'running'].includes(status)
  }
  return status === 'running'
}
const isHTTPResource = resource => ['http', 'http_direct'].includes(resource.type)
const isTCPResource = resource => resource.type === 'kubernetes_tcp'
const sourceKey = resource => isHTTPResource(resource) ? 'http' : String(resource.clusterId || '')
const clusterName = (id, fallback) => clusters.value.find(item => item.id === String(id))?.name || fallback || 'Kubernetes 集群'
function serviceDescription(item) {
  const resource = item.resource
  if (isDirectResource(resource)) return 'HTTP URL · 直连'
  const source = isHTTPResource(resource) ? 'HTTP URL · 代理' : clusterName(resource.clusterId, item.clusterName || resource.clusterName)
  if (isTCPResource(resource)) return resource.onDemand ? `${source} · TCP 转发 · 按需连接` : `${source} · TCP 转发`
  return resource.onDemand ? `${source} · 按需连接` : source
}
const clusterOptions = computed(() => [...new Set(resources.value.map(item => sourceKey(item.resource)).filter(Boolean))].map(id => {
  const item = resources.value.find(item => sourceKey(item.resource) === id)
  return { id, name: id === 'http' ? 'HTTP URL' : clusterName(id, item?.clusterName || item?.resource.clusterName) }
}))
const currentFolderResources = computed(() => resources.value.filter(item => Number(item.resource.folder_id || 0) === activeFolderId.value))
const runningCount = computed(() => currentFolderResources.value.filter(item => serviceStatus(item) === 'running').length)
const visibleResources = computed(() => currentFolderResources.value.filter(item => {
  const resource = item.resource
  const text = [resource.name, resource.targetURL, resource.namespace, resource.service, isHTTPResource(resource) ? 'HTTP URL' : clusterName(resource.clusterId, item.clusterName || resource.clusterName)].join(' ').toLowerCase()
  return (!keyword.value.trim() || text.includes(keyword.value.trim().toLowerCase())) && (!clusterFilter.value || sourceKey(resource) === String(clusterFilter.value)) && (!statusFilter.value || serviceStatus(item) === statusFilter.value)
}))
const statusInfo = item => serviceStatuses[serviceStatus(item)] || { label: '状态未知', type: 'info' }
const errorText = error => error.response?.data?.message || error.message || '请求失败'
let disposed = false, inactive = false, activatedOnce = false, loadingPage = false, refreshingResources = false, resourceRefreshTimer, suppressOpenUntil = 0

async function load(background = false, reloadFolders = true) {
  if (disposed || loadingPage || refreshingResources) return
  loadingPage = true
  loading.value = background !== true
  loadError.value = ''
  try {
    const config = { silentError: background === true }
    const [info, result] = await Promise.all([getServicePortalInfo(config), getPortalResources(config)])
    if (disposed) return
    Object.assign(portalInfo, info.data)
    resources.value = result.data || []
    available.value = true
    // Editing metadata must not hold the already loaded directory and services
    // behind a page-wide loading mask.
    refreshDatasources()
    if (reloadFolders) refreshFolders()
  } catch (error) {
    available.value = false
    loadError.value = `服务访问后端暂不可用：${errorText(error)}`
  } finally {
    loadingPage = false
    loading.value = false
  }
}

async function refreshDatasources() {
  if (!canManage.value) { datasources.value = []; return }
  if (disposed || loadingDatasources.value) return
  loadingDatasources.value = true
  datasourceLoadError.value = ''
  try {
    const result = await getDatasources({ page: 1, page_size: 1000, type: 'k8s', status: 1 })
    if (!disposed) datasources.value = result.data?.list || []
  } catch {
    if (!disposed) { datasources.value = []; datasourceLoadError.value = 'Kubernetes 接入加载失败，请刷新后重试。' }
  } finally { loadingDatasources.value = false }
}
async function refreshFolders() {
  try { await folderTreeRef.value?.refresh() }
  catch { /* Folder API errors are displayed by the request interceptor. */ }
}

watch(() => route.query.folder, () => { activeFolderId.value = folderIdFromRoute() })
function navigateFolder(id) {
  if (Number(id) !== folderIdFromRoute()) router.push({ path: route.path, query: { ...route.query, folder: Number(id) ? String(id) : undefined } })
}
function resetFolderRoute() {
  activeFolderId.value = 0
  if (route.query.folder !== undefined) router.replace({ path: route.path, query: { ...route.query, folder: undefined } })
}
function goToKubernetesSources() { router.push('/integrations/sources?type=k8s') }
async function refreshResources(background = false) {
  if (disposed || loadingPage || refreshingResources) return
  refreshingResources = true
  try {
    const result = await getPortalResources({ silentError: background === true })
    if (!disposed) { resources.value = result.data || []; available.value = true; loadError.value = '' }
  } catch {
    if (!disposed) { resources.value = []; available.value = false; loadError.value = '无法刷新服务状态或权限，正在自动重试。' }
  } finally { refreshingResources = false }
}
function pollResources() {
  if (disposed || inactive || document.hidden || busy.id || savingResource.value) return
  if (available.value) refreshResources(true)
  else load(true)
}
async function handleCreateCommand(command) {
  if (!canCreate.value || !available.value) return
  if (command === 'folder') {
    try { await folderTreeRef.value?.createFolder() } catch { /* The API displays errors. */ }
  } else await editResource()
}
function startResourceDrag(event, item) {
  if (!canManageResource(item) || !available.value || busy.id) { event.preventDefault(); return }
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData('application/x-guanlan-service-id', item.resource.id)
}
function finishResourceDrag() { suppressOpenUntil = Date.now() + 250 }
async function moveResourceToFolder({ id, folderId }) {
  const item = resources.value.find(item => item.resource.id === id)
  if (!item || !canManageResource(item) || Number(item.resource.folder_id || 0) === folderId || busy.id) return
  busy.id = id
  busy.action = 'move'
  try { await movePortalResource(id, folderId); ElMessage.success('服务已移动'); await refreshResources() }
  catch { /* The API checks both source and destination folder permissions. */ }
  finally { busy.id = ''; busy.action = '' }
}

async function openResource(item) {
  if (!canOpenService(item) || Date.now() < suppressOpenUntil) return
  const popup = window.open('about:blank', '_blank')
  if (!popup) { ElMessage.warning('请允许浏览器打开新窗口后重试'); return }
  popup.opener = null
  opening.value = item.resource.id
  try {
    let url
    if (isDirectResource(item.resource)) {
      url = httpTargetURL(item.resource.targetURL)
    } else {
      const { data } = await createPortalAccess(item.resource.id)
      url = isTCPResource(item.resource) ? tcpBrowserURL(data?.url) : portalAccessURL(data?.url)
    }
    if (!popup.closed) popup.location.replace(url)
  } catch (error) {
    popup.close()
    if (!error.response) ElMessage.error(error.message || '无法打开服务')
  } finally {
    opening.value = ''
  }
}

function tcpBrowserURL(value) {
  const url = new URL(httpTargetURL(value))
  if (url.protocol !== 'https:' || !url.port) throw new Error('服务返回了无效的 TCP 地址')
  return url.href
}

async function changeRunning(item, running) {
  busy.id = item.resource.id
  busy.action = running ? 'start' : 'stop'
  try {
    await setPortalResourceRunning(item.resource.id, running)
    const enabledMessage = item.resource.onDemand ? '已启用，访问时建立连接' : '已启用，正在自动连接'
    ElMessage.success(running ? enabledMessage : '已停用，正在关闭访问通道')
  } catch { /* API errors are displayed by the request interceptor. */ }
  finally { await load(); busy.id = ''; busy.action = '' }
}

async function removeResource(item) {
  try {
    const message = isDirectResource(item.resource) ? `删除直连服务「${item.resource.name}」？` : `删除服务「${item.resource.name}」并关闭其访问连接？`
    await ElMessageBox.confirm(message, '删除服务', { type: 'warning' })
    busy.id = item.resource.id
    busy.action = 'delete'
    await deletePortalResource(item.resource.id)
    ElMessage.success('服务已删除')
    await load()
  } catch { /* Cancellation and request errors require no further action. */ }
  finally { busy.id = ''; busy.action = '' }
}

const resourceVisible = ref(false), resourceFormRef = ref(), savingResource = ref(false)
const blankResource = () => ({ id: '', type: clusters.value.length ? 'kubernetes' : 'http_direct', targetURL: '', name: '', clusterId: '', namespace: '', service: '', port: '', publicPort: nextPort(), path: '/', onDemand: true, folder_id: activeFolderId.value })
const resourceForm = reactive(blankResource())
const resourceAccessOptions = computed(() => isHTTPResource(resourceForm) ? [
  { value: 'http', label: '观澜代理', description: '由门户连接目标 URL，并校验观澜访问权限。' },
  { value: 'http_direct', label: '浏览器直连', description: '由当前电脑访问目标 URL，登录由目标应用处理。' },
] : [
  { value: 'kubernetes', label: 'HTTP 代理', description: '适用于 Nacos 等 HTTP 服务，由观澜校验访问权限。' },
  { value: 'kubernetes_tcp', label: 'TCP 转发', description: '适用于 Argo CD 等使用自身 HTTPS 的服务。打开服务时临时放行当前 IP，登录由应用处理。' },
])
const namespaces = ref([]), services = ref([]), discoveryLoading = ref(false), discoveryError = ref('')
let discoveryVersion = 0
const namespaceOptions = computed(() => [...new Set([...namespaces.value, resourceForm.namespace].filter(Boolean))])
const serviceOptions = computed(() => {
  const options = isTCPResource(resourceForm) ? services.value.filter(item => item.type !== 'ExternalName') : services.value
  if (resourceForm.service && !services.value.some(item => item.name === resourceForm.service)) {
    return [...options, { name: resourceForm.service, ports: [] }]
  }
  return options
})
const selectedService = computed(() => services.value.find(item => item.name === resourceForm.service))
const isExternalName = computed(() => selectedService.value?.type === 'ExternalName')
const portOptions = computed(() => {
  const ports = (selectedService.value?.ports || []).filter(port => !port.protocol || port.protocol === 'TCP').map(port => ({ value: String(port.port), label: port.name ? `${port.name} (${port.port})` : String(port.port) }))
  if (resourceForm.port && !ports.some(port => port.value === resourceForm.port)) ports.push({ value: resourceForm.port, label: resourceForm.port })
  return ports
})
const resourceRules = computed(() => ({
  name: [{ required: true, whitespace: true, message: '请输入服务名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择接入方式', trigger: 'change' }],
  ...(isHTTPResource(resourceForm) ? {
    targetURL: [{ validator: (_rule, value, callback) => { try { httpTargetURL(value); callback() } catch (error) { callback(error) } }, trigger: 'blur' }],
  } : {
    clusterId: [{ required: true, message: '请选择集群', trigger: 'change' }],
    namespace: [{ required: true, message: '请选择命名空间', trigger: 'change' }],
    service: [
      { required: true, message: '请选择 Service', trigger: 'change' },
      { validator: (_rule, _value, callback) => callback(isTCPResource(resourceForm) && isExternalName.value ? new Error('TCP 转发需要匹配 Pod 的 Service') : undefined), trigger: 'change' },
    ],
    port: [
      { required: true, message: '请选择或输入服务端口', trigger: 'change' },
      { validator: (_rule, value, callback) => callback(!isExternalName.value || (/^[0-9]+$/.test(String(value)) && Number(value) >= 1 && Number(value) <= 65535) ? undefined : new Error('ExternalName 请输入 1–65535 的数字 TCP 端口')), trigger: 'change' },
    ],
    path: [{ validator: (_rule, value, callback) => callback(value.startsWith('/') && !value.startsWith('//') && !/[\r\n\\]/.test(value) ? undefined : new Error('请输入以 / 开头的站内路径')), trigger: 'blur' }],
  }),
  ...(isDirectResource(resourceForm) ? {} : { publicPort: [{ validator: (_rule, value, callback) => callback(Number.isInteger(value) && value >= portalInfo.minPort && value <= portalInfo.maxPort ? undefined : new Error('请输入范围内的整数端口')), trigger: 'change' }], }),
}))

function nextPort() {
  const used = new Set(resources.value.map(item => item.resource.publicPort))
  for (let port = portalInfo.minPort; port <= portalInfo.maxPort; port++) if (!used.has(port)) return port
  return undefined
}
function resetDiscovery() { discoveryVersion++; namespaces.value = []; services.value = []; discoveryError.value = ''; discoveryLoading.value = false }
async function editResource(item) {
  if (item ? !canManageResource(item) : !canCreate.value) return
  resetDiscovery()
  Object.assign(resourceForm, blankResource(), item?.resource || {})
  if (item) {
    resourceForm.type = item.resource.type || 'kubernetes'
    resourceForm.onDemand = item.resource.onDemand === true
  }
  resourceForm.port = String(resourceForm.port || '')
  resourceVisible.value = true
  resourceFormRef.value?.clearValidate()
  if (!isHTTPResource(resourceForm) && resourceForm.clusterId) await discover(true)
}
function resourceSourceChanged(source) {
  resourceForm.type = source === 'url' ? 'http_direct' : 'kubernetes'
  resourceTypeChanged()
}
function resourceTypeChanged() {
  if (!isHTTPResource(resourceForm) && resourceForm.clusterId) {
    if (isTCPResource(resourceForm) && isExternalName.value) {
      resourceForm.service = ''
      resourceForm.port = ''
    }
    resourceFormRef.value?.clearValidate()
    return
  }
  resetDiscovery()
  Object.assign(resourceForm, { targetURL: isHTTPResource(resourceForm) ? resourceForm.targetURL : '', clusterId: '', namespace: '', service: '', port: '', path: '/' })
  resourceFormRef.value?.clearValidate()
}
async function discover(includeServices = false) {
  const version = ++discoveryVersion
  const clusterId = resourceForm.clusterId
  const namespace = resourceForm.namespace
  if (!clusterId) return
  discoveryLoading.value = true
  discoveryError.value = ''
  try {
    const result = await getPortalNamespaces(clusterId)
    if (version !== discoveryVersion) return
    namespaces.value = result.data || []
    if (includeServices && namespace) {
      const result = await getPortalServices(clusterId, namespace)
      if (version !== discoveryVersion) return
      services.value = result.data || []
    }
  } catch (error) {
    if (version === discoveryVersion) discoveryError.value = `无法读取集群资源：${errorText(error)}`
  } finally {
    if (version === discoveryVersion) discoveryLoading.value = false
  }
}
async function clusterChanged() { resourceForm.namespace = ''; resourceForm.service = ''; resourceForm.port = ''; namespaces.value = []; services.value = []; await discover() }
async function namespaceChanged() {
  resourceForm.service = ''; resourceForm.port = ''; services.value = []
  await discover(true)
}
function serviceChanged() {
  resourceForm.port = ''
  const ports = portOptions.value
  if (ports.length === 1) resourceForm.port = ports[0].value
  if (!resourceForm.name) resourceForm.name = resourceForm.service
}
async function retryDiscovery() { await discover(true) }
async function saveResource() {
  if (!await resourceFormRef.value?.validate().catch(() => false)) return
  if (!isDirectResource(resourceForm) && resources.value.some(item => item.resource.id !== resourceForm.id && item.resource.publicPort === resourceForm.publicPort)) { ElMessage.warning('该访问端口已被其他服务使用'); return }
  savingResource.value = true
  try {
    const { id, type, name, targetURL, clusterId, namespace, service, port, publicPort, path, folder_id } = resourceForm
    const target = isHTTPResource(resourceForm) ? { targetURL } : { clusterId, namespace, service, port, path }
    const direct = isDirectResource(resourceForm)
    const onDemand = !direct && resourceForm.onDemand === true
    await savePortalResource({ id, type, name: name.trim(), ...target, publicPort: direct ? 0 : publicPort, onDemand, folder_id: Number(folder_id || 0) })
    resourceVisible.value = false
    if (direct) ElMessage.success('直连服务已保存，可直接打开')
    else if (id) ElMessage.success(onDemand ? '服务配置已保存，启用后可按需连接' : '服务配置已保存，启用后自动连接')
    else ElMessage.success(onDemand ? '服务已保存，访问时建立连接' : '服务已保存，正在自动连接')
    await load()
  } catch { /* API errors are displayed by the request interceptor. */ }
  finally { savingResource.value = false }
}

function startResourcePolling() {
  if (resourceRefreshTimer) return
  resourceRefreshTimer = window.setInterval(pollResources, 5000)
  document.addEventListener('visibilitychange', pollResources)
}
function stopResourcePolling() {
  window.clearInterval(resourceRefreshTimer)
  resourceRefreshTimer = null
  document.removeEventListener('visibilitychange', pollResources)
}
onMounted(() => {
  // DashboardFolderTree loads its own directories on its first mount.
  load(false, false)
  startResourcePolling()
})
onActivated(() => {
  inactive = false
  activeFolderId.value = folderIdFromRoute()
  startResourcePolling()
  if (activatedOnce) { pollResources(); refreshFolders(); refreshDatasources() }
  activatedOnce = true
})
onDeactivated(() => {
  inactive = true
  resourceVisible.value = false
  stopResourcePolling()
})
onBeforeUnmount(() => {
  disposed = true
  stopResourcePolling()
})
</script>

<style src="../../styles/service-dashboard.css"></style>

<style scoped>
.service-dashboard { --service-card-columns: repeat(auto-fill, minmax(340px, 1fr)); --service-card-height: 220px; display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.service-intro { display: flex; min-height: 36px; align-items: center; gap: 9px; padding: 7px 12px; color: #536f91; background: #eaf5ff; font-size: 11px; }
.intro-icon { flex: none; color: #409eff; font-size: 14px; }
.intro-message { min-width: 0; overflow: hidden; color: #718096; text-overflow: ellipsis; white-space: nowrap; }
.page-toolbar, .service-toolbar, .service-filter-actions, .toolbar-actions { display: flex; align-items: center; gap: 12px; }
.service-toolbar { justify-content: space-between; padding: 0; border: 0; background: transparent; }
.service-filter-actions { min-width: 0; flex-wrap: wrap; }
.toolbar-actions { flex: none; }
.create-caret { margin-left: 6px; }
.service-search { width: 320px; }
.filter-select { width: 150px; }
.service-count { color: var(--ops-muted); font-size: 13px; white-space: nowrap; }
.dashboard-browser { min-height: 360px; }
.dashboard-content { min-width: 0; flex: 1; }
.folder-hint { margin: 20px 0; color: var(--ops-muted); font-size: 13px; text-align: center; }
.service-grid, .service-dashboard :deep(.folder-grid) { grid-template-columns: var(--service-card-columns); }
.service-grid { min-height: 70px; }
.service-card, .service-dashboard :deep(.folder-card) { height: var(--service-card-height); box-sizing: border-box; border-radius: 12px; }
.service-dashboard :deep(.folder-card) { --service-accent: #c98a24; --service-tint: #fff6e6; border-top: 3px solid var(--service-accent); background: var(--ops-surface); }
.service-card { --service-accent: #3977ee; --service-tint: #edf3ff; background: var(--ops-surface); border-color: var(--ops-border-soft); border-top: 3px solid var(--service-accent); border-radius: 12px; }
.service-card--proxy { --service-accent: #0e9388; --service-tint: #e8f7f4; }
.service-card--direct { --service-accent: #8054cf; --service-tint: #f2edfc; }
.service-card.is-openable { cursor: pointer; }
.service-card.is-openable:hover { border-color: var(--service-accent); box-shadow: 0 6px 20px color-mix(in srgb, var(--service-accent) 12%, transparent); }
.service-card:focus-visible { outline: 2px solid var(--ops-primary, #409eff); outline-offset: 3px; }
.service-card .dashboard-entry-main, .folder-service-open { flex: 0 0 auto; min-height: 72px; gap: 10px; padding: 16px 18px 12px; }
.folder-service-open { cursor: pointer; }
.folder-service-open:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -3px; }
.folder-service-arrow { flex: none; color: var(--ops-muted); }
.service-icon { width: 32px; height: 32px; flex: 0 0 32px; color: var(--service-accent); background: var(--service-tint); border-radius: 8px; }
.service-icon img { width: 20px; height: 20px; object-fit: contain; }
.service-icon .el-icon { font-size: 20px; }
.dashboard-entry-name { color: var(--ops-text); line-height: 22px; }
.dashboard-entry-description { color: var(--ops-muted); white-space: nowrap; display: block; text-overflow: ellipsis; }
.service-card .el-tag { flex-shrink: 0; border-radius: 6px; }
.service-card--direct .el-tag--info { color: var(--service-accent); background: var(--service-tint); border-color: transparent; }
.service-card-content { flex: 1; min-width: 0; min-height: 0; overflow: hidden; }
.service-details { box-sizing: border-box; margin: 0; padding: 0 18px 20px; display: grid; grid-template-columns: minmax(0, 1fr); gap: 8px; font-size: 12px; line-height: 18px; }
.service-details > div { display: flex; min-width: 0; align-items: baseline; gap: 12px; }
.service-details dt { color: var(--ops-muted); width: 66px; flex-shrink: 0; }
.service-details dd { flex: 1; margin: 0; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.resource-error { margin: 0; padding: 0 18px 14px; font-size: 12px; color: var(--ops-danger); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.service-card-footer { flex: 0 0 48px; min-height: 48px; gap: 14px; margin-top: auto; background: var(--ops-surface-soft); border-color: var(--ops-border-soft); padding: 10px 18px; }
.service-card-footer > .el-button { margin-left: auto; border-radius: 7px; }
.resource-actions { margin-right: auto; display: flex; align-items: center; gap: 12px; }
.resource-actions .el-button + .el-button { margin-left: 0; }
.el-form .el-select { width: 100%; }
.el-form .el-input-number { width: 170px; }
.field-help { display: block; color: var(--ops-muted); font-size: 12px; line-height: 1.7; margin-top: 8px; width: 100%; }
.resource-access-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); grid-auto-rows: 1fr; align-items: stretch; gap: 8px; width: 100%; }
.resource-access-option.el-radio.is-bordered { display: flex; align-items: flex-start; height: auto; min-width: 0; margin: 0; padding: 10px; white-space: normal; }
.resource-access-option :deep(.el-radio__input) { margin-top: 3px; }
.resource-access-option :deep(.el-radio__label) { min-width: 0; line-height: 1.5; }
.resource-access-option strong { display: block; }
.resource-access-description { display: block; margin-top: 4px; color: var(--ops-muted); font-size: 12px; }
@media (max-width: 480px) { .resource-access-options { grid-template-columns: 1fr; } }
.discovery-error { margin-bottom: 18px; }
@media (max-width: 900px) { .service-toolbar { align-items: stretch; flex-direction: column; } .service-filter-actions { width: 100%; } .toolbar-actions { justify-content: flex-end; flex-wrap: wrap; } }
@media (max-width: 760px) { .service-dashboard { --service-card-columns: minmax(0, 1fr); } .service-search { width: 100%; } .filter-select { flex: 1; } .service-card .dashboard-entry-main { flex-wrap: wrap; } .intro-message { white-space: normal; } }
</style>

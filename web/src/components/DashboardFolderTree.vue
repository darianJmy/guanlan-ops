<template>
  <section v-if="modelValue || childFolders.length" class="folder-browser" aria-label="仪表盘目录">
    <div v-if="modelValue" class="folder-breadcrumb">
      <button
        v-for="item in breadcrumbs"
        :key="item.id"
        type="button"
        class="breadcrumb-item"
        :class="{ current: item.id === modelValue, 'drop-target': dragOverFolderId === item.id }"
        @click="selectFolder(item.id)"
        @dragover.prevent.stop="dragOverFolderId = item.id"
        @dragleave.stop="leaveDropTarget(item.id)"
        @drop.prevent.stop="dropItem($event, item.id)"
      >{{ item.name }}</button>
      <div class="current-actions">
        <el-tooltip v-if="canManageCurrent" content="配置目录权限">
          <el-button link :icon="Lock" aria-label="配置目录权限" @click="openPermissions" v-permission="[permissionCodes.edit]" />
        </el-tooltip>
        <el-tooltip v-if="canManageCurrent" content="重命名当前目录">
          <el-button link :icon="Edit" aria-label="重命名目录" @click="renameFolder" v-permission="[permissionCodes.edit]" />
        </el-tooltip>
        <el-tooltip v-if="canManageCurrent" content="删除当前目录">
          <el-button link type="danger" :icon="Delete" aria-label="删除目录" @click="removeFolder" v-permission="[permissionCodes.delete]" />
        </el-tooltip>
      </div>
    </div>

    <div v-if="childFolders.length" class="folder-grid dashboard-entry-grid">
      <article
        v-for="folder in childFolders"
        :key="folder.id"
        class="folder-card dashboard-entry-card"
        :class="{ 'drop-target': dragOverFolderId === folder.id }"
        @dragover.prevent.stop="folderCanEdit(folder) && (dragOverFolderId = folder.id)"
        @dragleave.stop="leaveDropTarget(folder.id)"
        @drop.prevent.stop="dropItem($event, folder.id)"
      >
        <slot name="folder-card" :folder="folder" :open="() => selectFolder(folder.id)">
          <button type="button" class="folder-open dashboard-entry-main" @click="selectFolder(folder.id)">
            <span class="folder-icon dashboard-entry-icon"><el-icon><Folder /></el-icon></span>
            <strong class="dashboard-entry-name" :title="folder.name">{{ folder.name }}</strong>
            <el-icon class="folder-arrow"><ArrowRight /></el-icon>
          </button>
          <footer class="folder-footer dashboard-entry-footer" aria-hidden="true"></footer>
        </slot>
      </article>
    </div>
  </section>

  <el-dialog v-model="permissionVisible" :title="`目录权限 · ${permissionFolderName}`" width="min(620px, calc(100vw - 32px))" :close-on-click-modal="false">
    <div v-loading="permissionLoading" class="folder-permission-form">
      <el-form label-position="top">
        <el-form-item label="权限模式">
          <el-radio-group v-model="permissionMode">
            <el-radio-button label="inherit">继承上级</el-radio-button>
            <el-radio-button label="restricted">限制访问</el-radio-button>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <el-table v-if="permissionMode === 'restricted'" :data="permissionRoles" max-height="360" stripe>
        <el-table-column prop="name" label="角色" min-width="190">
          <template #default="{ row }">
            <strong>{{ row.name }}</strong>
            <code class="role-code">{{ row.code }}</code>
          </template>
        </el-table-column>
        <el-table-column label="目录权限" width="180">
          <template #default="{ row }">
            <el-select v-model="row.access_level" :disabled="row.code === 'admin'" aria-label="目录权限">
              <el-option label="无权限" value="none" />
              <el-option label="查看" value="view" />
              <el-option label="编辑" value="edit" />
              <el-option label="管理" value="manage" />
            </el-select>
          </template>
        </el-table-column>
      </el-table>
    </div>
    <template #footer>
      <el-button @click="permissionVisible = false">取消</el-button>
      <el-button type="primary" :loading="permissionSaving" :disabled="permissionLoading" @click="savePermissions">保存</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowRight, Delete, Edit, Folder, Lock } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dashboardFolderDropId, dashboardFolderPermissions } from '../utils/dashboardFolders'
import {
  createDashboardFolder, deleteDashboardFolder, getDashboardFolderPermissions,
  getDashboardFolders, updateDashboardFolder, updateDashboardFolderPermissions,
} from '../api/dashboard'

const props = defineProps({
  scope: { type: String, required: true },
  modelValue: { type: Number, default: 0 },
})
const emit = defineEmits(['update:modelValue', 'folders-change', 'move-item', 'invalid-folder', 'permissions-change'])
const permissionCodes = computed(() => dashboardFolderPermissions(props.scope))
const folders = ref([])
const dragOverFolderId = ref(null)
const permissionVisible = ref(false)
const permissionLoading = ref(false)
const permissionSaving = ref(false)
const permissionMode = ref('inherit')
const permissionRoles = ref([])
const permissionFolderName = ref('')
const selectedFolder = computed(() => folders.value.find(item => Number(item.id) === props.modelValue))
const canManageCurrent = computed(() => selectedFolder.value?.access_level === 'manage')
const canEditCurrent = computed(() => !props.modelValue || ['edit', 'manage'].includes(selectedFolder.value?.access_level))
const childFolders = computed(() => folders.value
  .filter(item => Number(item.parent_id || 0) === props.modelValue)
  .sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0) || a.name.localeCompare(b.name)))
const breadcrumbs = computed(() => {
  const result = [{ id: 0, name: props.scope === 'services' ? '全部服务' : '全部仪表盘' }]
  if (!props.modelValue) return result
  const path = []
  const seen = new Set()
  let current = selectedFolder.value
  while (current && !seen.has(current.id)) {
    seen.add(current.id)
    path.unshift({ id: Number(current.id), name: current.name })
    current = folders.value.find(item => Number(item.id) === Number(current.parent_id))
  }
  return result.concat(path)
})

onMounted(fetchFolders)
watch(() => props.scope, () => {
  emit('update:modelValue', 0)
  fetchFolders()
})

async function fetchFolders() {
  const res = await getDashboardFolders(props.scope)
  folders.value = res.data || []
  if (props.modelValue && !folders.value.some(item => Number(item.id) === props.modelValue)) emit('invalid-folder')
  emit('folders-change', folders.value)
}

function selectFolder(id) {
  emit('update:modelValue', Number(id) || 0)
}

function folderCanEdit(folder) {
  return ['edit', 'manage'].includes(folder?.access_level)
}

async function promptName(title, value = '') {
  try {
    const result = await ElMessageBox.prompt('目录名称', title, {
      inputValue: value,
      inputPlaceholder: '请输入目录名称',
      inputValidator: input => {
        const name = String(input || '').trim()
        if (!name) return '请输入目录名称'
        if ([...name].length > 100) return '目录名称不能超过 100 个字符'
        return true
      },
      confirmButtonText: '保存',
      cancelButtonText: '取消',
    })
    return String(result.value || '').trim()
  } catch {
    return ''
  }
}

async function createFolder() {
  if (!canEditCurrent.value) {
    ElMessage.error('没有当前目录的编辑权限')
    return
  }
  const name = await promptName(props.modelValue ? '新建子目录' : '新建目录')
  if (!name) return
  const res = await createDashboardFolder({ scope: props.scope, parent_id: props.modelValue, name, sort: folders.value.length })
  ElMessage.success('目录已创建')
  await fetchFolders()
  if (res.data?.id) selectFolder(Number(res.data.id))
}

function leaveDropTarget(id) {
  if (dragOverFolderId.value === id) dragOverFolderId.value = null
}

function dropItem(event, folderID) {
  dragOverFolderId.value = null
  const target = Number(folderID) ? folders.value.find(item => Number(item.id) === Number(folderID)) : null
  if (target && !folderCanEdit(target)) return
  const id = dashboardFolderDropId(props.scope, event.dataTransfer)
  if (id !== null) emit('move-item', { id, folderId: Number(folderID) || 0 })
}

defineExpose({ createFolder, refresh: fetchFolders })

async function openPermissions() {
  if (!selectedFolder.value || !canManageCurrent.value) return
  permissionVisible.value = true
  permissionLoading.value = true
  permissionFolderName.value = selectedFolder.value.name
  try {
    const res = await getDashboardFolderPermissions(selectedFolder.value.id)
    permissionMode.value = res.data?.permission_mode || 'inherit'
    permissionRoles.value = (res.data?.roles || []).map(role => ({
      ...role,
      access_level: role.code === 'admin' ? 'manage' : (role.access_level || 'none'),
    }))
  } finally {
    permissionLoading.value = false
  }
}

async function savePermissions() {
  if (!selectedFolder.value) return
  permissionSaving.value = true
  try {
    const permissions = permissionMode.value === 'restricted'
      ? permissionRoles.value
        .filter(role => role.code !== 'admin' && role.access_level !== 'none')
        .map(role => ({ role_id: role.id, access_level: role.access_level }))
      : []
    await updateDashboardFolderPermissions(selectedFolder.value.id, {
      permission_mode: permissionMode.value,
      permissions,
    })
    ElMessage.success('目录权限已保存')
    permissionVisible.value = false
    await fetchFolders()
    emit('permissions-change')
  } finally {
    permissionSaving.value = false
  }
}

async function renameFolder() {
  if (!selectedFolder.value) return
  const name = await promptName('重命名目录', selectedFolder.value.name)
  if (!name || name === selectedFolder.value.name) return
  await updateDashboardFolder(selectedFolder.value.id, { name, parent_id: selectedFolder.value.parent_id, sort: selectedFolder.value.sort })
  ElMessage.success('目录已更新')
  await fetchFolders()
}

async function removeFolder() {
  if (!selectedFolder.value) return
  try {
    await ElMessageBox.confirm(`确定删除空目录「${selectedFolder.value.name}」吗？`, '删除目录', { type: 'warning' })
  } catch {
    return
  }
  await deleteDashboardFolder(selectedFolder.value.id)
  selectFolder(Number(selectedFolder.value.parent_id) || 0)
  ElMessage.success('目录已删除')
  await fetchFolders()
}
</script>

<style scoped>
.folder-browser { margin-bottom: 16px; }
.folder-breadcrumb { display: flex; min-height: 34px; align-items: center; gap: 4px; margin-bottom: 12px; border-bottom: 1px solid var(--ops-border-soft, #eef1f5); }
.breadcrumb-item { position: relative; padding: 6px 18px 6px 8px; color: var(--ops-primary, #409eff); cursor: pointer; background: transparent; border: 0; font-size: 12px; }
.breadcrumb-item::after { position: absolute; right: 4px; color: #b4bcc8; content: '/'; }
.breadcrumb-item.current { color: var(--ops-text, #303133); font-weight: 600; }
.breadcrumb-item.drop-target { background: #ddecff; outline: 1px dashed var(--ops-primary, #409eff); outline-offset: -1px; }
.current-actions { display: flex; align-items: center; gap: 2px; margin-left: auto; }
.current-actions :deep(.el-button) { width: 26px; height: 26px; margin: 0; }
.folder-card { border-color: var(--ops-border, #e5e9f0); }
.folder-open { cursor: pointer; }
.folder-icon { color: #e6a23c; background: #fff6e6; font-size: 26px; }
.folder-open strong { min-width: 0; flex: 1; }
.folder-arrow { flex: none; color: #a7b0bd; }
.folder-footer { flex: 0 0 38px; }
.folder-permission-form { min-height: 120px; }
.folder-permission-form :deep(.el-form-item) { margin-bottom: 16px; }
.folder-permission-form :deep(.el-select) { width: 100%; }
.role-code { margin-left: 8px; color: var(--ops-muted, #909399); font-size: 11px; font-weight: 400; }
</style>

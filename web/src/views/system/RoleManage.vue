<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="角色列表搜索">
        <el-input v-model="keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索角色名称" placeholder="搜索角色名称" @input="handleKeywordInput" @keyup.enter="handleSearch" @clear="handleSearch" />
        <el-select v-model="statusFilter" class="filter-select" clearable aria-label="按角色状态筛选" placeholder="状态" @change="handleSearch">
          <el-option label="已启用" :value="1" />
          <el-option label="已停用" :value="0" />
        </el-select>
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions">
        <el-dropdown trigger="click" :disabled="!selectedRows.length || !hasBatchPermission || !!batchAction" @command="handleBatchCommand">
          <el-button :disabled="!selectedRows.length || !hasBatchPermission || !!batchAction" :loading="!!batchAction">批量操作<el-icon class="batch-operation-arrow"><ArrowDown /></el-icon></el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="can('role:edit')" command="enable">批量启用</el-dropdown-item>
              <el-dropdown-item v-if="can('role:edit')" command="disable">批量停用</el-dropdown-item>
              <el-dropdown-item v-if="can('role:delete')" command="delete" divided>批量删除</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="handleAdd" v-permission="['role:create']">创建</el-button>
      </div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" row-key="id" stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="name" label="角色名称" min-width="170" show-overflow-tooltip><template #default="{ row }"><strong class="role-name">{{ row.name }}</strong></template></el-table-column>
        <el-table-column prop="code" label="角色编码" min-width="160" show-overflow-tooltip><template #default="{ row }"><code class="role-code">{{ row.code }}</code></template></el-table-column>
        <el-table-column prop="description" label="描述" min-width="240" show-overflow-tooltip><template #default="{ row }"><span class="secondary-text">{{ row.description || '—' }}</span></template></el-table-column>
        <el-table-column label="状态" width="76" align="center"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
        <el-table-column label="创建时间" width="145"><template #default="{ row }"><span class="secondary-text">{{ formatTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="操作" width="140" fixed="right" align="center">
          <template #default="{ row }"><div class="row-actions">
            <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑角色" @click="handleEdit(row)" v-permission="['role:edit']" /></el-tooltip>
            <el-tooltip content="分配权限" placement="top"><el-button link type="primary" :icon="Lock" aria-label="分配权限" @click="handleAssignPerm(row)" /></el-tooltip>
            <el-tooltip content="分配菜单" placement="top"><el-button link type="primary" :icon="Menu" aria-label="分配菜单" @click="handleAssignMenu(row)" /></el-tooltip>
            <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除角色" @click="handleDelete(row)" v-permission="['role:delete']" /></el-tooltip>
          </div></template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div v-if="total > 0" class="task-pagination">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize" :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next" @current-change="handlePageChange" @size-change="handleSizeChange" />
      </div>
    </el-card>

    <!-- 新增/编辑弹窗 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑角色' : '创建角色'" width="500px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="角色名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="角色编码" prop="code">
          <el-input v-model="form.code" :disabled="isEdit" placeholder="如: admin, operator" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.description" type="textarea" />
        </el-form-item>
        <el-form-item label="状态">
          <el-switch v-model="form.status" :active-value="1" :inactive-value="0" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 分配权限弹窗 -->
    <el-dialog v-model="permVisible" title="分配权限" width="500px">
      <el-checkbox-group v-model="checkedPermIds">
        <div v-for="p in allPermissions" :key="p.id" style="margin-bottom:8px">
          <el-checkbox :value="p.id" :label="p.id">
            {{ p.name }} <span style="color:#909399;font-size:12px">({{ p.code }})</span>
          </el-checkbox>
        </div>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="permVisible = false">取消</el-button>
        <el-button type="primary" @click="submitPermAssign">确认</el-button>
      </template>
    </el-dialog>

    <!-- 分配菜单弹窗 -->
    <el-dialog v-model="menuVisible" title="分配菜单" width="400px">
      <el-tree ref="menuTreeRef" :data="allMenus" show-checkbox node-key="id"
        :props="{ label: 'name', children: 'children' }" :default-checked-keys="checkedMenuIds" />
      <template #footer>
        <el-button @click="menuVisible = false">取消</el-button>
        <el-button type="primary" @click="submitMenuAssign">确认</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ArrowDown, Delete, Edit, Lock, Menu, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { getRoles, createRole, updateRole, deleteRole, assignPermissions, assignMenus } from '../../api/role'
import { getPermissions } from '../../api/permission'
import { getMenus } from '../../api/menu'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '../../stores/user'

const userStore = useUserStore()
const loading = ref(false)
const saving = ref(false)
const allRoles = ref([])
const keyword = ref('')
const statusFilter = ref('')
const page = ref(1)
const pageSize = ref(10)
const selectedRows = ref([])
const batchAction = ref('')
let keywordSearchTimer

const filteredRoles = computed(() => {
  const normalizedKeyword = keyword.value.trim().toLowerCase()
  return allRoles.value.filter((role) => {
    const matchesKeyword = !normalizedKeyword || String(role.name || '').toLowerCase().includes(normalizedKeyword)
    const matchesStatus = statusFilter.value === '' || role.status === statusFilter.value
    return matchesKeyword && matchesStatus
  })
})
const total = computed(() => filteredRoles.value.length)
const tableData = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredRoles.value.slice(start, start + pageSize.value)
})
const hasFilters = computed(() => keyword.value.trim() !== '' || statusFilter.value !== '')
const hasBatchPermission = computed(() => can('role:edit') || can('role:delete'))

const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref()
const form = reactive({ id: null, name: '', code: '', description: '', status: 1 })
const rules = {
  name: [{ required: true, message: '请输入角色名称', trigger: 'blur' }],
  code: [{ required: true, message: '请输入角色编码', trigger: 'blur' }],
}

const permVisible = ref(false)
const allPermissions = ref([])
const checkedPermIds = ref([])
const currentRole = ref(null)

const menuVisible = ref(false)
const allMenus = ref([])
const checkedMenuIds = ref([])
const menuTreeRef = ref()

onMounted(fetchData)
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))

async function fetchData() {
  loading.value = true
  try {
    const res = await getRoles()
    allRoles.value = Array.isArray(res.data) ? res.data : []
    normalizePage()
  } finally {
    loading.value = false
  }
}

function can(permission) { return userStore.permissions.includes(permission) }
function handleKeywordInput() { clearTimeout(keywordSearchTimer); keywordSearchTimer = setTimeout(handleSearch, 300) }
function handleSearch() { clearTimeout(keywordSearchTimer); page.value = 1 }
function clearFilters() { keyword.value = ''; statusFilter.value = ''; page.value = 1 }
function handlePageChange(value) { page.value = value }
function handleSizeChange() { page.value = 1 }
function handleSelectionChange(rows) { selectedRows.value = rows }
function normalizePage() { page.value = Math.min(page.value, Math.max(1, Math.ceil(total.value / pageSize.value))) }

async function handleBatchCommand(command) {
  const rows = [...selectedRows.value]
  if (!rows.length) return
  const actionLabel = { enable: '启用', disable: '停用', delete: '删除' }[command]
  await ElMessageBox.confirm(`确定批量${actionLabel}选中的 ${rows.length} 个角色吗？`, '批量操作', { type: command === 'delete' ? 'warning' : 'info' })
  batchAction.value = command
  try {
    if (command === 'delete') {
      await Promise.all(rows.map((row) => deleteRole(row.id)))
    } else {
      const status = command === 'enable' ? 1 : 0
      await Promise.all(rows.map((row) => updateRole(row.id, { name: row.name, description: row.description, status })))
    }
    ElMessage.success(`批量${actionLabel}成功`)
    selectedRows.value = []
    await fetchData()
  } finally {
    batchAction.value = ''
  }
}

function handleAdd() {
  isEdit.value = false
  Object.assign(form, { id: null, name: '', code: '', description: '', status: 1 })
  dialogVisible.value = true
}

function handleEdit(row) {
  isEdit.value = true
  Object.assign(form, { id: row.id, name: row.name, code: row.code, description: row.description, status: row.status })
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (isEdit.value) {
      await updateRole(form.id, { name: form.name, description: form.description, status: form.status })
      ElMessage.success('更新成功')
    } else {
      await createRole(form)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    await fetchData()
  } finally {
    saving.value = false
  }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除角色「${row.name}」吗？`, '提示', { type: 'warning' })
  await deleteRole(row.id)
  ElMessage.success('删除成功')
  await fetchData()
}

async function handleAssignPerm(row) {
  currentRole.value = row
  const res = await getPermissions()
  allPermissions.value = res.data
  checkedPermIds.value = row.permissions?.map((p) => p.id) || []
  permVisible.value = true
}

async function submitPermAssign() {
  await assignPermissions(currentRole.value.id, checkedPermIds.value)
  ElMessage.success('权限分配成功')
  permVisible.value = false
  await fetchData()
}

async function handleAssignMenu(row) {
  currentRole.value = row
  const res = await getMenus()
  allMenus.value = res.data
  checkedMenuIds.value = row.menus?.map((m) => m.id) || []
  menuVisible.value = true
}

async function submitMenuAssign() {
  const ids = menuTreeRef.value.getCheckedKeys()
  await assignMenus(currentRole.value.id, ids)
  ElMessage.success('菜单分配成功')
  menuVisible.value = false
  await fetchData()
}

function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
</script>

<style scoped>
.task-page { --ink: var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 220px 110px auto; gap: 8px; }
.task-search, .filter-select { width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.page-header-actions :deep(.el-button + .el-button), .row-actions :deep(.el-button + .el-button) { margin-left: 0; }
.batch-operation-arrow { margin-left: 6px; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }
.role-name { color: var(--ops-text); font-size: 13px; font-weight: 400; }
.role-code { padding: 2px 5px; color: #4d5d73; background: var(--ops-surface-soft); font-size: 12px; }
.secondary-text { color: var(--ops-muted); font-size: 12px; }
.row-actions { display: flex; align-items: center; justify-content: center; gap: 4px; }
.row-actions .el-button { width: 26px; height: 26px; }
:deep(.el-table .el-tag) { border-radius: 0; }
:deep(.el-table th.el-table__cell) { color: var(--ops-muted); background: var(--ops-surface-soft); font-weight: 600; }
@media (max-width: 900px) { .task-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; }.task-pagination { overflow-x: auto; justify-content: flex-start; } }
@media (max-width: 560px) { .task-toolbar { align-items: stretch; flex-direction: column; }.filter-bar { grid-template-columns: 1fr 1fr; }.task-search { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; } }
</style>

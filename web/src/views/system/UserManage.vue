<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="用户列表搜索">
          <el-input
            v-model="keyword"
            class="task-search"
            :prefix-icon="Search"
            placeholder="搜索用户名、昵称或邮箱"
            aria-label="搜索用户"
            clearable
            @input="handleKeywordInput"
            @keyup.enter="handleSearch"
            @clear="handleSearch"
          />
          <el-select v-model="statusFilter" class="filter-select filter-status" clearable aria-label="按用户状态筛选" placeholder="状态" @change="handleSearch">
            <el-option label="已启用" :value="1" />
            <el-option label="已停用" :value="0" />
          </el-select>
          <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions">
          <el-dropdown trigger="click" :disabled="!selectedRows.length || !hasBatchPermission || !!batchAction" @command="handleBatchCommand">
            <el-button :disabled="!selectedRows.length || !hasBatchPermission || !!batchAction" :loading="!!batchAction">
              批量操作
              <el-icon class="batch-operation-arrow"><ArrowDown /></el-icon>
            </el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="can('user:edit')" command="enable">批量启用</el-dropdown-item>
                <el-dropdown-item v-if="can('user:edit')" command="disable">批量停用</el-dropdown-item>
                <el-dropdown-item v-if="can('user:delete')" command="delete" divided>批量删除</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button>
          <el-button type="primary" :icon="Plus" @click="handleAdd" v-permission="['user:create']">创建</el-button>
      </div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" row-key="id" stripe @selection-change="handleSelectionChange">
        <el-table-column type="selection" width="48" />
        <el-table-column label="用户名" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">
            <strong class="user-name">{{ row.username }}</strong>
          </template>
        </el-table-column>
        <el-table-column label="昵称" min-width="140" show-overflow-tooltip>
          <template #default="{ row }"><span class="secondary-text">{{ row.nickname || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="邮箱" min-width="160" show-overflow-tooltip>
          <template #default="{ row }"><span class="secondary-text">{{ row.email || '—' }}</span></template>
        </el-table-column>
        <el-table-column label="角色" min-width="120">
          <template #default="{ row }">
            <div v-if="row.roles?.length" class="role-tags"><el-tag v-for="role in row.roles" :key="role.id" size="small" effect="light">{{ role.name }}</el-tag></div>
            <span v-else class="empty-value">未分配</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="76" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">{{ row.status === 1 ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="145">
          <template #default="{ row }"><span class="secondary-text">{{ formatTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right" align="center">
          <template #default="{ row }">
            <div class="row-actions">
              <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑用户" @click="handleEdit(row)" v-permission="['user:edit']" /></el-tooltip>
              <el-tooltip content="分配角色" placement="top"><el-button link type="primary" :icon="UserFilled" aria-label="分配角色" @click="handleAssignRole(row)" /></el-tooltip>
              <el-tooltip content="重置密码" placement="top"><el-button link type="warning" :icon="Key" aria-label="重置密码" @click="handleResetPwd(row)" /></el-tooltip>
              <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除用户" @click="handleDelete(row)" v-permission="['user:delete']" /></el-tooltip>
            </div>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>

      <div v-if="total > 0" class="task-pagination">
        <el-pagination v-model:current-page="page" v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50, 100]" :total="total" layout="total, sizes, prev, pager, next"
          @current-change="fetchData" @size-change="handleSizeChange" />
      </div>
    </el-card>

    <!-- 新增/编辑弹窗 -->
    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑用户' : '创建用户'" width="500px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :disabled="isEdit" />
        </el-form-item>
        <el-form-item label="密码" prop="password" v-if="!isEdit">
          <el-input v-model="form.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="昵称" prop="nickname">
          <el-input v-model="form.nickname" />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="手机号">
          <el-input v-model="form.phone" />
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

    <!-- 分配角色弹窗 -->
    <el-dialog v-model="roleVisible" title="分配角色" width="400px">
      <el-checkbox-group v-model="checkedRoles" class="role-options">
        <div v-for="role in allRoles" :key="role.id">
          <el-checkbox :value="role.id" :label="role.id">{{ role.name }}</el-checkbox>
        </div>
      </el-checkbox-group>
      <template #footer>
        <el-button @click="roleVisible = false">取消</el-button>
        <el-button type="primary" @click="submitRoleAssign">确认</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { getUsers, createUser, updateUser, deleteUser, assignUserRoles, resetPassword } from '../../api/user'
import { getRoles } from '../../api/role'
import { useUserStore } from '../../stores/user'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Delete, Edit, Key, Plus, Refresh, Search, UserFilled } from '@element-plus/icons-vue'

const loading = ref(false)
const userStore = useUserStore()
const saving = ref(false)
const tableData = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const keyword = ref('')
const statusFilter = ref('')
const selectedRows = ref([])
const batchAction = ref('')
const hasFilters = computed(() => keyword.value.trim() !== '' || statusFilter.value !== '')
const hasBatchPermission = computed(() => can('user:edit') || can('user:delete'))

const dialogVisible = ref(false)
const isEdit = ref(false)
const formRef = ref()
const form = reactive({ id: null, username: '', password: '', nickname: '', email: '', phone: '', status: 1 })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, min: 6, message: '密码至少6位', trigger: 'blur' }],
}

const roleVisible = ref(false)
const currentUser = ref(null)
const allRoles = ref([])
const checkedRoles = ref([])
let keywordSearchTimer

onMounted(fetchData)
onBeforeUnmount(() => clearTimeout(keywordSearchTimer))

async function fetchData() {
  loading.value = true
  try {
    const res = await getUsers({ page: page.value, page_size: pageSize.value, keyword: keyword.value, status: statusFilter.value === '' ? undefined : statusFilter.value })
    tableData.value = res.data?.list || []
    total.value = res.data?.total || 0
  } finally {
    loading.value = false
  }
}

function handleKeywordInput() {
  clearTimeout(keywordSearchTimer)
  keywordSearchTimer = setTimeout(handleSearch, 300)
}

function handleSearch() {
  clearTimeout(keywordSearchTimer)
  page.value = 1
  fetchData()
}

function clearFilters() {
  keyword.value = ''
  statusFilter.value = ''
  handleSearch()
}

function handleSizeChange() {
  page.value = 1
  fetchData()
}

function handleSelectionChange(rows) { selectedRows.value = rows }
function can(permission) { return userStore.permissions.includes(permission) }

async function handleBatchCommand(command) {
  const rows = [...selectedRows.value]
  if (!rows.length) return
  const actionLabel = { enable: '启用', disable: '停用', delete: '删除' }[command]
  await ElMessageBox.confirm(`确定批量${actionLabel}选中的 ${rows.length} 个用户吗？`, '批量操作', { type: command === 'delete' ? 'warning' : 'info' })
  batchAction.value = command
  try {
    if (command === 'delete') {
      await Promise.all(rows.map((row) => deleteUser(row.id)))
    } else {
      const status = command === 'enable' ? 1 : 0
      await Promise.all(rows.map((row) => updateUser(row.id, { nickname: row.nickname, email: row.email, phone: row.phone, status })))
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
  Object.assign(form, { id: null, username: '', password: '', nickname: '', email: '', phone: '', status: 1 })
  dialogVisible.value = true
}

function handleEdit(row) {
  isEdit.value = true
  Object.assign(form, { id: row.id, username: row.username, nickname: row.nickname, email: row.email, phone: row.phone, status: row.status })
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (isEdit.value) {
      await updateUser(form.id, { nickname: form.nickname, email: form.email, phone: form.phone, status: form.status })
      ElMessage.success('更新成功')
    } else {
      await createUser(form)
      ElMessage.success('创建成功')
    }
    dialogVisible.value = false
    await fetchData()
  } finally { saving.value = false }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除用户「${row.username}」吗？`, '提示', { type: 'warning' })
  await deleteUser(row.id)
  ElMessage.success('删除成功')
  fetchData()
}

async function handleAssignRole(row) {
  currentUser.value = row
  const res = await getRoles()
  allRoles.value = res.data
  checkedRoles.value = row.roles?.map((r) => r.id) || []
  roleVisible.value = true
}

async function submitRoleAssign() {
  await assignUserRoles(currentUser.value.id, checkedRoles.value)
  ElMessage.success('角色分配成功')
  roleVisible.value = false
  fetchData()
}

async function handleResetPwd(row) {
  await ElMessageBox.confirm(`确定重置「${row.username}」的密码为 123456 吗？`, '提示', { type: 'warning' })
  await resetPassword(row.id)
  ElMessage.success('密码已重置')
}

function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
</script>

<style scoped>
.task-page { --ink: var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 260px 110px auto; gap: 8px; }
.task-search, .filter-select { width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.page-header-actions :deep(.el-button + .el-button), .row-actions :deep(.el-button + .el-button) { margin-left: 0; }
.batch-operation-arrow { margin-left: 6px; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.task-pagination { display: flex; justify-content: flex-end; margin-top: 16px; }
.row-actions { display: flex; align-items: center; gap: 4px; }
.user-name { color: var(--ops-text); font-size: 13px; font-weight: 400; }
.secondary-text, .empty-value { color: var(--ops-muted); font-size: 12px; }
.role-tags { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; }
.role-tags :deep(.el-tag), :deep(.el-table .el-tag) { border-radius: 0; }
.row-actions { justify-content: center; }
.row-actions .el-button { width: 26px; height: 26px; }
.role-options { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
.role-options > div { min-width: 0; }
:deep(.el-table th.el-table__cell) { color: var(--ops-muted); background: var(--ops-surface-soft); font-weight: 600; }
@media (max-width: 900px) { .task-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; }.task-pagination { overflow-x: auto; justify-content: flex-start; } }
@media (max-width: 560px) { .task-toolbar { align-items: stretch; flex-direction: column; }.filter-bar { grid-template-columns: 1fr 1fr; }.task-search { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; }.role-options { grid-template-columns: 1fr; } }
</style>

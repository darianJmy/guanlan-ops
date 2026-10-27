<template>
  <div class="task-page">
    <header class="task-toolbar">
      <div class="filter-bar" role="search" aria-label="菜单列表搜索">
        <el-input v-model="keyword" class="task-search" :prefix-icon="Search" clearable aria-label="搜索菜单" placeholder="搜索名称、路由或权限标识" />
        <el-select v-model="typeFilter" class="filter-select" clearable aria-label="按菜单类型筛选" placeholder="类型">
          <el-option label="目录" :value="1" /><el-option label="菜单" :value="2" /><el-option label="按钮" :value="3" />
        </el-select>
        <el-select v-model="statusFilter" class="filter-select" clearable aria-label="按菜单状态筛选" placeholder="状态">
          <el-option label="已启用" :value="1" /><el-option label="已停用" :value="0" />
        </el-select>
        <el-button :disabled="!hasFilters" @click="clearFilters">清空</el-button>
      </div>
      <div class="page-header-actions"><el-button :icon="Refresh" :loading="loading" @click="fetchData">刷新</el-button></div>
    </header>

    <el-card class="task-card" shadow="never">
      <el-table :data="tableData" v-loading="loading" row-key="id" stripe default-expand-all>
        <el-table-column prop="name" label="菜单名称" min-width="150" show-overflow-tooltip />
        <el-table-column prop="icon" label="图标" width="55" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.icon"><component :is="row.icon" /></el-icon>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路由路径" min-width="130" show-overflow-tooltip><template #default="{ row }"><code class="menu-code">{{ row.path || '—' }}</code></template></el-table-column>
        <el-table-column prop="component" label="组件路径" min-width="160" show-overflow-tooltip><template #default="{ row }"><span class="secondary-text">{{ row.component || '—' }}</span></template></el-table-column>
        <el-table-column prop="permission" label="权限标识" min-width="125" show-overflow-tooltip><template #default="{ row }"><code class="menu-code">{{ row.permission || '—' }}</code></template></el-table-column>
        <el-table-column label="类型" width="70" align="center">
          <template #default="{ row }">
            <el-tag size="small" effect="light" :type="row.type === 1 ? '' : row.type === 2 ? 'success' : 'warning'">
              {{ { 1: '目录', 2: '菜单', 3: '按钮' }[row.type] }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="sort" label="排序" width="60" align="center" />
        <el-table-column label="状态" width="70" align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small" effect="light">
              {{ row.status === 1 ? '启用' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="80" fixed="right" align="center">
          <template #default="{ row }">
            <div class="row-actions">
              <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑菜单" @click="handleEdit(row)" /></el-tooltip>
              <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除菜单" @click="handleDelete(row)" /></el-tooltip>
            </div>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" title="编辑菜单" width="550px" :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="上级菜单">
          <el-tree-select v-model="form.parent_id" :data="menuOptions" :props="{ label: 'name', value: 'id', children: 'children' }"
            placeholder="无（根菜单）" clearable check-strictly style="width:100%" />
        </el-form-item>
        <el-form-item label="菜单名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-radio-group v-model="form.type">
            <el-radio :value="1">目录</el-radio>
            <el-radio :value="2">菜单</el-radio>
            <el-radio :value="3">按钮</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="路由路径" v-if="form.type !== 3">
          <el-input v-model="form.path" placeholder="如 /system/users" />
        </el-form-item>
        <el-form-item label="组件路径" v-if="form.type === 2">
          <el-input v-model="form.component" placeholder="如 system/UserManage" />
        </el-form-item>
        <el-form-item label="图标" v-if="form.type !== 3">
          <el-input v-model="form.icon" placeholder="Element Plus 图标名，如 User" />
        </el-form-item>
        <el-form-item label="权限标识" v-if="form.type !== 1">
          <el-input v-model="form.permission" placeholder="如 user:list" />
        </el-form-item>
        <el-form-item label="排序">
          <el-input-number v-model="form.sort" :min="0" />
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
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { Delete, Edit, Refresh, Search } from '@element-plus/icons-vue'
import { getMenus, updateMenu, deleteMenu, getMenuOptions } from '../../api/menu'
import { ElMessage, ElMessageBox } from 'element-plus'

const loading = ref(false)
const saving = ref(false)
const menuTree = ref([])
const menuOptions = ref([])
const keyword = ref('')
const typeFilter = ref('')
const statusFilter = ref('')
const hasFilters = computed(() => keyword.value.trim() !== '' || typeFilter.value !== '' || statusFilter.value !== '')
const tableData = computed(() => filterMenuTree(menuTree.value))

const dialogVisible = ref(false)
const formRef = ref()
const form = reactive({ id: null, parent_id: null, name: '', type: 2, path: '', component: '', icon: '', permission: '', sort: 0, status: 1 })
const rules = {
  name: [{ required: true, message: '请输入菜单名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择类型', trigger: 'change' }],
}

onMounted(fetchData)

async function fetchData() {
  loading.value = true
  try {
    const res = await getMenus()
    menuTree.value = res.data || []
    const optRes = await getMenuOptions()
    menuOptions.value = optRes.data
  } finally {
    loading.value = false
  }
}

function filterMenuTree(items) {
  const normalizedKeyword = keyword.value.trim().toLowerCase()
  return (items || []).reduce((result, item) => {
    const filteredChildren = filterMenuTree(item.children || [])
    const matchesKeyword = !normalizedKeyword || [item.name, item.path, item.component, item.permission].some((value) => String(value || '').toLowerCase().includes(normalizedKeyword))
    const matchesType = typeFilter.value === '' || item.type === typeFilter.value
    const matchesStatus = statusFilter.value === '' || item.status === statusFilter.value
    if ((matchesKeyword && matchesType && matchesStatus) || filteredChildren.length) {
      result.push({ ...item, children: filteredChildren })
    }
    return result
  }, [])
}

function clearFilters() { keyword.value = ''; typeFilter.value = ''; statusFilter.value = '' }

function handleEdit(row) {
  Object.assign(form, {
    id: row.id, parent_id: row.parent_id === 0 ? null : row.parent_id,
    name: row.name, type: row.type, path: row.path, component: row.component,
    icon: row.icon, permission: row.permission, sort: row.sort, status: row.status,
  })
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  const payload = { ...form, parent_id: form.parent_id || 0 }
  saving.value = true
  try {
    await updateMenu(form.id, payload)
    ElMessage.success('更新成功')
    dialogVisible.value = false
    await fetchData()
  } finally {
    saving.value = false
  }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除菜单「${row.name}」吗？`, '提示', { type: 'warning' })
  await deleteMenu(row.id)
  ElMessage.success('删除成功')
  await fetchData()
}
</script>

<style scoped>
.task-page { --ink: var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.task-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.filter-bar { display: grid; min-width: 0; flex: 0 0 auto; grid-template-columns: 260px 100px 100px auto; gap: 8px; }
.task-search, .filter-select { width: 100%; }
.page-header-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.task-card { min-width: 0; border-color: var(--border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.task-card :deep(.el-card__body) { padding: 16px; }
.menu-code { color: #4d5d73; font-size: 12px; }
.secondary-text { color: var(--ops-muted); font-size: 12px; }
.row-actions { display: flex; align-items: center; justify-content: center; gap: 4px; }
.row-actions .el-button { width: 26px; height: 26px; }
:deep(.el-table .el-tag) { border-radius: 0; }
:deep(.el-table th.el-table__cell) { color: var(--ops-muted); background: var(--ops-surface-soft); font-weight: 600; }
@media (max-width: 1050px) { .task-toolbar { align-items: stretch; flex-wrap: wrap; }.filter-bar { width: 100%; flex: 1 1 100%; }.page-header-actions { margin-left: auto; } }
@media (max-width: 620px) { .task-toolbar { flex-direction: column; }.filter-bar { grid-template-columns: repeat(2, minmax(0, 1fr)); }.task-search { grid-column: 1 / -1; }.page-header-actions { justify-content: flex-end; } }
</style>

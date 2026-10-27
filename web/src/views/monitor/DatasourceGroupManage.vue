<template>
  <div class="group-page">
    <div class="group-layout">
      <aside class="group-sidebar" aria-label="实例分组">
        <div class="sidebar-heading">
          <el-icon><Grid /></el-icon><strong>实例分组</strong><span class="group-count">{{ groups.length }}</span>
          <el-button v-permission="['datasource:create']" text size="small" :icon="Plus" aria-label="新增实例分组" title="新增分组" @click="openDialog()" />
        </div>
        <div class="group-search">
          <el-input v-model="keyword" :prefix-icon="Search" clearable size="small" aria-label="搜索实例分组" placeholder="搜索分组" />
        </div>
        <nav v-loading="loading" class="group-menu" aria-label="选择实例分组">
          <div v-for="group in filteredGroups" :key="group.id" class="group-item" :class="{ active: selectedGroupId === group.id }">
            <button type="button" class="group-select" :aria-current="selectedGroupId === group.id ? 'true' : undefined" :title="group.path || group.name" @click="selectedGroupId = group.id">
              <span class="group-name">{{ group.name }}</span><span class="member-count">{{ group.scope_member_count ?? group.member_count }}</span>
            </button>
            <span class="group-node-actions">
              <button v-permission="['datasource:edit']" type="button" class="group-node-action" :aria-label="`重命名分组 ${group.name}`" title="重命名分组" @click="openDialog(group)"><el-icon><Edit /></el-icon></button>
              <button v-permission="['datasource:delete']" type="button" class="group-node-action danger" :aria-label="`删除分组 ${group.name}`" title="删除分组" @click="removeGroup(group)"><el-icon><Minus /></el-icon></button>
            </span>
          </div>
          <p v-if="!loading && !filteredGroups.length" class="sidebar-empty">{{ keyword.trim() ? '未找到匹配分组' : '暂无分组，点击 + 创建' }}</p>
        </nav>
      </aside>

      <section v-loading="loading" class="group-content" aria-label="分组实例列表">
        <DatasourceManage :group-id="selectedGroupId" :group-name="selectedGroup?.name || ''" :refresh-key="refreshKey" require-group @changed="loadGroups" />
      </section>
    </div>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑分组' : '创建分组'" width="min(380px, 92vw)" :close-on-click-modal="false" :close-on-press-escape="!saving" :show-close="!saving" destroy-on-close @opened="groupNameInput?.focus()">
      <el-form ref="formRef" :model="form" :rules="formRules" @submit.prevent="saveGroup">
        <el-form-item prop="name">
          <el-input ref="groupNameInput" v-model="form.name" placeholder="输入分组名称，如生产环境、eu" aria-label="分组名称" maxlength="128" :disabled="saving" />
        </el-form-item>
      </el-form>
      <template #footer><el-button :disabled="saving" @click="dialogVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveGroup">{{ form.id ? '保存' : '创建' }}</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Edit, Grid, Minus, Plus, Search } from '@element-plus/icons-vue'
import { createDatasourceGroup, deleteDatasourceGroup, getDatasourceGroups, updateDatasourceGroup } from '../../api/datasourceGroup'
import DatasourceManage from './DatasourceManage.vue'

const groups = ref([]), keyword = ref(''), selectedGroupId = ref(null)
const loading = ref(false), saving = ref(false), dialogVisible = ref(false), formRef = ref(), groupNameInput = ref()
const refreshKey = ref(0)
const emptyForm = () => ({ id: null, parent_id: 0, name: '', description: '', datasource_ids: [] })
const form = reactive(emptyForm())
const formRules = {
  name: [{ required: true, whitespace: true, message: '请输入数据源分组名称', trigger: 'blur' }],
}
const filteredGroups = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  return groups.value.filter(group => `${group.name} ${group.description || ''}`.toLowerCase().includes(query))
})
const selectedGroup = computed(() => groups.value.find(group => group.id === selectedGroupId.value))
onMounted(loadGroups)

async function loadGroups() {
  loading.value = true
  try {
    groups.value = (await getDatasourceGroups()).data || []
    if (!groups.value.some(group => group.id === selectedGroupId.value)) selectedGroupId.value = groups.value[0]?.id ?? null
  }
  finally { loading.value = false }
}
async function refresh() { refreshKey.value++; await loadGroups() }
function openDialog(row) {
  Object.assign(form, row ? { id: row.id, parent_id: row.parent_id || 0, name: row.name, description: row.description || '', datasource_ids: [...(row.datasource_ids || [])] } : emptyForm())
  dialogVisible.value = true
}
async function saveGroup() {
  if (saving.value) return
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = { parent_id: form.parent_id || 0, name: form.name.trim(), description: form.description.trim(), datasource_ids: form.datasource_ids }
    if (form.id) await updateDatasourceGroup(form.id, payload)
    else {
      const response = await createDatasourceGroup(payload)
      selectedGroupId.value = response.data.id
      keyword.value = ''
    }
    ElMessage.success(form.id ? '数据源分组已更新' : '数据源分组已创建')
    dialogVisible.value = false
    await refresh()
  } finally { saving.value = false }
}
async function removeGroup(row) {
  if ((row.scope_member_count ?? row.member_count ?? row.datasource_ids?.length ?? 0) > 0) { ElMessage.warning('分组内存在数据源，不能删除'); return }
  if (row.rule_count > 0) { ElMessage.warning(`该数据源分组仍关联 ${row.rule_count} 条告警规则，请先解除关联。`); return }
  if (!await ElMessageBox.confirm(`确认删除数据源分组“${row.name}”？`, '删除数据源分组', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }).then(() => true).catch(() => false)) return
  await deleteDatasourceGroup(row.id)
  ElMessage.success('数据源分组已删除')
  await loadGroups()
}
</script>

<style scoped>
.group-node-actions { display: inline-flex; align-items: center; gap: 1px; flex-shrink: 0; }
.group-node-action { display: inline-flex; align-items: center; justify-content: center; width: 20px; height: 22px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--ops-subtle); cursor: pointer; font-size: 12px; }
.group-node-action:hover:not(:disabled) { color: var(--ops-primary); background: var(--ops-primary-soft); }
.group-node-action.danger:hover { color: var(--ops-danger); background: color-mix(in srgb, var(--ops-danger) 10%, var(--ops-surface)); }
.group-node-action:disabled { opacity: .3; cursor: not-allowed; }
.group-node-action:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -2px; }

.group-layout { display: flex; align-items: stretch; gap: 16px; min-height: 480px; }
.group-sidebar { width: 220px; flex-shrink: 0; display: flex; flex-direction: column; max-height: calc(100vh - 120px); min-height: 320px; border: 1px solid var(--ops-border-soft); border-radius: 0; background: var(--ops-surface); }
.sidebar-heading { display: flex; align-items: center; gap: 6px; padding: 10px 12px; color: var(--ops-muted); font-size: 14px; }
.sidebar-heading strong { color: var(--ops-text); font-size: 13px; }
.sidebar-heading .el-button { margin-left: auto; padding: 4px; }
.group-count, .member-count { flex-shrink: 0; padding: 2px 6px; border-radius: 3px; background: var(--ops-surface-soft); color: var(--ops-subtle); font-size: 11px; font-weight: 400; }
.group-search { padding: 0 8px 10px; }
.group-search :deep(.el-input__wrapper) { border-radius: 3px; }
.group-menu { flex: 1; overflow-y: auto; padding: 0 8px 8px; }
.group-item { display: flex; align-items: center; gap: 8px; box-sizing: border-box; width: 100%; margin-bottom: 6px; padding: 0 8px; border: 1px solid transparent; border-radius: 3px; background: transparent; color: var(--ops-text); cursor: pointer; text-align: left; font: inherit; font-size: 12px; transition: background .12s, border-color .12s; }
.group-select { display: flex; flex: 1; min-width: 0; align-items: center; gap: 8px; padding: 9px 4px; border: 0; background: transparent; color: inherit; text-align: left; font: inherit; cursor: pointer; }
.group-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-item > .el-icon { flex-shrink: 0; color: var(--ops-muted); }
.group-item:hover { background: var(--ops-surface-soft); border-color: var(--ops-border); }
.group-item.active { background: color-mix(in srgb, #a78753 13%, var(--ops-surface)); border-color: color-mix(in srgb, #a78753 45%, var(--ops-surface)); color: var(--ops-warning); font-weight: 500; }
.group-item.active > .el-icon { color: inherit; }
.group-select:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: -2px; }
.sidebar-empty { padding: 12px 4px; text-align: center; color: var(--ops-subtle); font-size: 12px; }
.group-content { flex: 1; min-width: 0; }
.group-content :deep(.datasource-table-card) { border: 1px solid var(--ops-border); border-radius: 8px; box-shadow: none; background: var(--ops-surface); }
.group-content :deep(.datasource-table-card > .el-card__body) { padding: 16px; }
.instance-name .el-icon { color: var(--ops-subtle); }
.pagination-wrap { display: flex; justify-content: flex-end; margin-top: 16px; overflow-x: auto; }
@media (max-width: 720px) {
  .group-layout { flex-direction: column; }
  .group-sidebar { width: 100%; min-height: 0; max-height: 260px; }
  .group-menu { min-height: 60px; }
  .pagination-wrap { justify-content: flex-start; }
}
</style>

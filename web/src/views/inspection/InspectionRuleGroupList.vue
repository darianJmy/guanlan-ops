<template>
  <div class="group-page">
    <div class="group-toolbar">
      <div class="search-area" role="search" aria-label="规则组筛选">
        <el-input v-model="keyword" clearable aria-label="搜索拨测规则组" placeholder="搜索规则组名称或描述" @keyup.enter="handleSearch" @clear="handleSearch">
          <template #append>
            <el-button aria-label="搜索" @click="handleSearch">
              <el-icon><Search /></el-icon>
            </el-button>
          </template>
        </el-input>
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadGroups">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openDrawer()" v-permission="['inspection:create']">创建</el-button>
      </div>
    </div>

    <section class="group-card">
      <el-table v-loading="loading" :data="groups" row-key="id">
        <el-table-column label="规则组名称" min-width="260">
          <template #default="{ row }">
            <button class="group-link" type="button" @click="enterGroup(row)">{{ row.name }}</button>
          </template>
        </el-table-column>
        <el-table-column label="任务数" width="110" align="center">
          <template #default="{ row }"><span class="count-badge">{{ row.task_count }}</span></template>
        </el-table-column>
        <el-table-column label="描述" min-width="420">
          <template #default="{ row }"><span class="description">{{ row.description || '' }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" :icon="Edit" aria-label="编辑规则组" @click="openDrawer(row)" v-permission="['inspection:edit']" />
            <el-button link type="danger" :icon="Delete" aria-label="删除规则组" @click="removeGroup(row)" v-permission="['inspection:delete']" :disabled="row.task_count > 0" />
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div class="pagination-wrap">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :page-sizes="[10, 20, 50]"
          :total="total"
          layout="total, sizes, prev, pager, next"
          @current-change="loadGroups"
          @size-change="handleSizeChange"
        />
      </div>
    </section>

    <el-drawer v-model="drawerVisible" class="group-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header><div class="drawer-header"><DrawerBackButton @click="drawerVisible = false" /><strong>{{ form.id ? '编辑规则组' : '创建规则组' }}</strong></div></template>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <el-form-item label="规则组名称" prop="name"><el-input v-model="form.name" maxlength="128" show-word-limit /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="5" maxlength="500" show-word-limit /></el-form-item>
        <div class="group-tip"><strong>规则组用于组织拨测任务</strong><span>创建后进入规则组，添加 HTTP、ICMP、TCP、证书到期或脚本拨测任务。</span></div>
      </el-form>
      <template #footer><el-button @click="drawerVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveGroup">保存</el-button></template>
    </el-drawer>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Delete, Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { createInspectionRuleGroup, deleteInspectionRuleGroup, getInspectionRuleGroups, updateInspectionRuleGroup } from '../../api/inspection'

const router = useRouter()
const groups = ref([]), keyword = ref(''), loading = ref(false), saving = ref(false), drawerVisible = ref(false), formRef = ref()
const page = ref(1), pageSize = ref(10), total = ref(0)
const emptyForm = () => ({ id: null, name: '', description: '' })
const form = reactive(emptyForm())
const rules = { name: [{ required: true, message: '请输入规则组名称', trigger: 'blur' }] }

onMounted(loadGroups)

async function loadGroups() {
  loading.value = true
  try {
    const data = (await getInspectionRuleGroups({ keyword: keyword.value.trim(), page: page.value, page_size: pageSize.value })).data || {}
    groups.value = data.list || []
    total.value = data.total || 0
  }
  finally { loading.value = false }
}

function handleSearch() { page.value = 1; loadGroups() }
function handleSizeChange() { page.value = 1; loadGroups() }

function enterGroup(row) { router.push(`/inspection/items/groups/${row.id}`) }
function openDrawer(row) {
  Object.assign(form, row ? { id: row.id, name: row.name, description: row.description || '' } : emptyForm())
  drawerVisible.value = true
}

async function saveGroup() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const payload = { name: form.name.trim(), description: form.description.trim() }
    if (form.id) await updateInspectionRuleGroup(form.id, payload)
    else {
      await createInspectionRuleGroup(payload)
      page.value = 1
    }
    ElMessage.success(form.id ? '规则组已更新' : '规则组已创建')
    drawerVisible.value = false
    await loadGroups()
  } finally { saving.value = false }
}

async function removeGroup(row) {
  const detail = row.task_count > 0 ? `规则组内还有 ${row.task_count} 条任务，请先移出或删除任务。` : `确认删除规则组“${row.name}”？`
  if (row.task_count > 0) { ElMessage.warning(detail); return }
  await ElMessageBox.confirm(detail, '删除规则组', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  await deleteInspectionRuleGroup(row.id)
  ElMessage.success('规则组已删除')
  if (groups.value.length === 1 && page.value > 1) page.value -= 1
  await loadGroups()
}

</script>

<style scoped>
.group-page { display: flex; flex-direction: column; gap: 16px; }
.group-toolbar, .search-area, .toolbar-actions { display: flex; align-items: center; gap: 8px; }
.group-toolbar { justify-content: space-between; }
.toolbar-actions :deep(.el-button + .el-button) { margin-left: 0; }
.search-area .el-input { width: 240px; }
.group-card { padding: 16px; border: 1px solid #e5e9f0; border-radius: 8px; background: var(--ops-surface); }
.pagination-wrap { display: flex; justify-content: flex-end; margin-top: 16px; }
.group-link { padding: 0; border: 0; background: transparent; color: var(--ops-primary); font-weight: 600; text-align: left; cursor: pointer; }.group-link:hover { color: var(--ops-primary); }.count-badge { display: inline-flex; min-width: 28px; height: 24px; align-items: center; justify-content: center; padding: 0 8px; border-radius: 12px; background: var(--ops-primary); color: #fff; font-size: 12px; }.description { color: var(--ops-muted); }
.drawer-header { display: flex; align-items: center; gap: 12px; }.drawer-header .el-button { margin-left: -10px; }.field-help { margin-top: 7px; color: #8a96a8; font-size: 11px; line-height: 1.6; }.group-tip { display: flex; flex-direction: column; gap: 6px; margin-top: 8px; padding: 14px; border: 1px solid #dbeafe; border-radius: 7px; background: #f5f9ff; }.group-tip strong { color: #344054; font-size: 13px; }.group-tip span { color: var(--ops-muted); font-size: 12px; line-height: 1.7; }
:global(.group-drawer.el-drawer) { border-radius: 0 !important; }
:global(.group-drawer .el-drawer__header) { margin-bottom: 0; padding: 16px 20px; border-bottom: 1px solid #e5e9f0; }:global(.group-drawer .el-drawer__body) { padding: 22px 24px; }:global(.group-drawer .el-drawer__footer) { padding: 12px 20px; border-top: 1px solid #e5e9f0; }
@media (max-width: 720px) { .group-toolbar { align-items: stretch; flex-direction: column; }.search-area { flex-wrap: wrap; }.search-area .el-input { width: min(100%, 240px); }.toolbar-actions { justify-content: flex-end; }.pagination-wrap { overflow-x: auto; justify-content: flex-start; } }
</style>

<template>
  <div class="page-container">
    <section class="script-workbench">
      <aside class="asset-sidebar" aria-label="脚本库分组">
        <div class="sidebar-heading"><el-icon aria-hidden="true"><Grid /></el-icon><strong>资产分组</strong></div>
        <nav class="asset-menu" aria-label="选择资产分组">
          <button type="button" class="asset-group" :class="{ active: assetTab === 'scripts' }" :aria-current="assetTab === 'scripts' ? 'true' : undefined" @click="selectAssetGroup('scripts')"><span class="asset-icon"><el-icon><Document /></el-icon></span><span class="asset-copy"><strong>在线脚本</strong><small>Shell · Python</small></span></button>
          <button type="button" class="asset-group" :class="{ active: assetTab === 'repositories' }" :aria-current="assetTab === 'repositories' ? 'true' : undefined" @click="selectAssetGroup('repositories')"><span class="asset-icon repository-icon"><el-icon><FolderOpened /></el-icon></span><span class="asset-copy"><strong>Git 仓库</strong><small>代码与版本管理</small></span></button>
        </nav>
      </aside>
      <section class="asset-content" :aria-label="assetTab === 'repositories' ? 'Git 仓库列表' : '在线脚本列表'">
        <header class="script-toolbar">
          <el-input v-model="keyword" clearable class="script-search" :aria-label="assetTab === 'repositories' ? '搜索 Git 仓库' : '搜索脚本'" :placeholder="assetTab === 'repositories' ? '搜索名称、地址或分支' : '搜索名称或描述'" @keyup.enter="handleSearch" @clear="handleSearch">
            <template #append>
              <el-button aria-label="搜索" @click="handleSearch"><el-icon><Search /></el-icon></el-button>
            </template>
          </el-input>
          <div class="script-toolbar-actions">
            <el-button :icon="Refresh" @click="loadScripts">刷新</el-button>
            <el-button type="primary" :icon="Plus" @click="openCreate" v-permission="createPermissions">创建</el-button>
          </div>
        </header>
      <div class="asset-list">
      <el-table v-if="assetTab !== 'repositories'" :data="pagedScripts" :row-key="rowKey" v-loading="loading">
        <el-table-column prop="name" label="脚本名称" min-width="190" show-overflow-tooltip />
        <el-table-column label="运行方式" width="110"><template #default="{ row }"><span class="runtime-label">{{ row.type === 'python' ? 'Python' : 'Shell' }}</span></template></el-table-column>
        <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="170"><template #default="{ row }">{{ formatTime(row.updated_at) }}</template></el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }"><div class="row-actions">
            <el-tooltip content="编辑" placement="top"><el-button link type="primary" :icon="Edit" aria-label="编辑脚本" @click="openEditor(row)" v-permission="['script:edit']" /></el-tooltip>
            <el-tooltip content="删除" placement="top"><el-button link type="danger" :icon="Delete" aria-label="删除脚本" @click="removeOne(row)" v-permission="['script:delete']" /></el-tooltip>
          </div></template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <el-table v-else :data="filteredRepositories" row-key="id" v-loading="loading">
        <el-table-column prop="name" label="仓库名称" min-width="180" show-overflow-tooltip />
        <el-table-column prop="repository_url" label="仓库地址" min-width="280" show-overflow-tooltip />
        <el-table-column prop="default_ref" label="默认分支/标签" width="150"><template #default="{ row }">{{ row.default_ref || '默认分支' }}</template></el-table-column>
        <el-table-column prop="credential_name" label="Git 认证" width="170" show-overflow-tooltip />
        <el-table-column label="同步状态" width="210"><template #default="{ row }"><div class="sync-state"><el-tag :type="syncState(row).type" size="small">{{ syncState(row).label }}</el-tag><small v-if="row.synced_revision">{{ row.synced_revision.slice(0, 12) }} · {{ formatTime(row.synced_at) }}</small></div></template></el-table-column>
        <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
        <el-table-column prop="updated_at" label="更新时间" width="170"><template #default="{ row }">{{ formatTime(row.updated_at) }}</template></el-table-column>
        <el-table-column label="操作" width="180" fixed="right"><template #default="{ row }"><el-button link type="primary" :loading="syncingRepositoryId === row.id" @click="syncRepository(row)">同步</el-button><el-button link type="primary" @click="openRepository(row)">编辑</el-button><el-button link type="danger" @click="removeRepository(row)">删除</el-button></template></el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <div v-if="assetTab !== 'repositories' && filteredScripts.length" class="table-pagination">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="filteredScripts.length"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @size-change="page = 1"
        />
      </div>
      </div>
      </section>
    </section>

    <el-drawer v-model="repositoryVisible" class="repository-drawer" size="min(460px, 94vw)" :close-on-click-modal="false">
      <template #header><div class="standard-drawer-header"><DrawerBackButton @click="repositoryVisible = false" /><div class="drawer-title">{{ repositoryForm.id ? '编辑 Git 仓库' : '创建 Git 仓库' }}<small>仓库由平台拉取，执行节点不会获得 Git 凭据</small></div></div></template>
      <el-form label-position="top">
        <el-form-item label="仓库名称" required><el-input v-model="repositoryForm.name" maxlength="128" /></el-form-item>
        <el-form-item label="仓库地址" required><el-input v-model="repositoryForm.repository_url" placeholder="https://git.example.com/team/ops.git" /></el-form-item>
        <el-form-item label="默认分支或标签"><el-input v-model="repositoryForm.default_ref" placeholder="留空使用仓库默认分支" /></el-form-item>
        <el-form-item label="Git 认证" required><el-select v-model="repositoryForm.credential_id" filterable placeholder="请选择系统设置中的 Git 认证"><el-option v-for="item in availableCredentials" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item>
        <el-form-item label="备注"><el-input v-model="repositoryForm.description" type="textarea" :rows="3" maxlength="500" /></el-form-item>
        <el-form-item label="启用状态"><el-switch v-model="repositoryForm.enabled" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="repositoryVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveRepository">保存</el-button></template>
    </el-drawer>

  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Document, Edit, FolderOpened, Grid, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { deleteScript, getScripts } from '../../api/script'
import { createGitRepository, deleteGitRepository, getGitRepositories, syncGitRepository, updateGitRepository } from '../../api/gitRepository'
import { getGitCredentials } from '../../api/gitCredential'

const router = useRouter()
const createPermissions = ['script:create']
const loading = ref(false), saving = ref(false), scripts = ref([])
const assetTab = ref('scripts'), repositories = ref([]), credentials = ref([]), repositoryVisible = ref(false), syncingRepositoryId = ref(null)
const blankRepository = () => ({ id:null, name:'', repository_url:'', default_ref:'', credential_id:null, description:'', enabled:true })
const repositoryForm = reactive(blankRepository())
const keyword = ref(''), page = ref(1), pageSize = ref(10)
const filteredScripts = computed(() => scripts.value)
const pagedScripts = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredScripts.value.slice(start, start + pageSize.value)
})
const filteredRepositories = computed(() => { const term=keyword.value.trim().toLowerCase(); return term ? repositories.value.filter(item => `${item.name} ${item.repository_url} ${item.default_ref} ${item.description}`.toLowerCase().includes(term)) : repositories.value })
const availableCredentials = computed(() => credentials.value.filter(item => item.status === 1 || item.id === repositoryForm.credential_id))
onMounted(loadScripts)

function selectAssetGroup(group) {
  if (assetTab.value === group) return
  assetTab.value = group
  keyword.value = ''
  handleSearch()
}

function handleSearch() { page.value = 1; loadScripts() }

async function loadScripts() {
  loading.value = true
  try {
    const [result, repositoryResult, credentialResult] = await Promise.all([getScripts({ keyword: keyword.value.trim() }), getGitRepositories(), getGitCredentials()])
    scripts.value = (result.data || []).map(item => ({ ...item, source: 'script' }))
    repositories.value = repositoryResult.data || []
    credentials.value = credentialResult.data || []
    scripts.value.sort((a, b) => new Date(b.updated_at) - new Date(a.updated_at))
    const lastPage = Math.max(1, Math.ceil(filteredScripts.value.length / pageSize.value))
    if (page.value > lastPage) page.value = lastPage
  } finally { loading.value = false }
}
function openCreate() { if (assetTab.value === 'repositories') openRepository(); else openEditor() }
function openRepository(row) { Object.assign(repositoryForm, blankRepository(), row ? { ...row, credential:'', enabled:row.status===1 } : {}); repositoryVisible.value=true }
async function saveRepository() { if(!repositoryForm.name.trim()) return ElMessage.warning('请输入仓库名称'); if(!repositoryForm.repository_url.trim()) return ElMessage.warning('请输入仓库地址'); if(!repositoryForm.credential_id) return ElMessage.warning('请选择 Git 认证'); saving.value=true; try { const payload={ name:repositoryForm.name.trim(), repository_url:repositoryForm.repository_url.trim(), default_ref:repositoryForm.default_ref.trim(), credential_id:Number(repositoryForm.credential_id), description:repositoryForm.description.trim(), status:repositoryForm.enabled?1:0 }; if(repositoryForm.id) await updateGitRepository(repositoryForm.id,payload); else await createGitRepository(payload); repositoryVisible.value=false; await loadScripts(); ElMessage.success('Git 仓库已保存') } finally { saving.value=false } }
async function syncRepository(row) { syncingRepositoryId.value=row.id; try { const response=await syncGitRepository(row.id); ElMessage.success(`同步成功：${response.data.revision.slice(0,12)}，制品 ${formatBytes(response.data.size)}`); await loadScripts() } finally { syncingRepositoryId.value=null } }
async function removeRepository(row) { await ElMessageBox.confirm(`确认删除 Git 仓库「${row.name}」？`,'删除确认',{type:'warning'}); await deleteGitRepository(row.id); await loadScripts(); ElMessage.success('Git 仓库已删除') }
function formatBytes(value) { const size=Number(value||0); return size>=1048576?`${(size/1048576).toFixed(1)} MB`:`${Math.ceil(size/1024)} KB` }
function syncState(row) { if(row.last_sync_error) return { type:row.synced_revision?'warning':'danger', label:row.synced_revision?'同步异常':'同步失败' }; return row.synced_revision?{type:'success',label:'已同步'}:{type:'info',label:'未同步'} }
function openEditor(row) {
  router.push(row ? `/automation/scripts/${row.id}/edit` : '/automation/scripts/create')
}
async function removeOne(row) {
  await ElMessageBox.confirm(`确认删除脚本「${row.name}」？`, '删除确认', { type: 'warning' })
  await deleteScript(row.id)
  ElMessage.success('脚本已删除'); await loadScripts()
}
function rowKey(row) { return `${row.source}:${row.id}` }
function formatTime(value) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' }
</script>

<style scoped>
.page-container { --ink:var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); height:100%; min-height:0; }
.script-workbench { display:flex; align-items:stretch; gap:16px; height:100%; min-height:0; }
.asset-sidebar { display:flex; flex-direction:column; width:220px; flex-shrink:0; min-height:0; align-self:stretch; box-sizing:border-box; padding:6px; border:1px solid var(--ops-border-soft); border-radius:12px; background:var(--ops-surface); }
.sidebar-heading { display:flex; align-items:center; gap:6px; box-sizing:border-box; height:52px; flex:0 0 52px; padding:0 12px; color:var(--ops-muted); font-size:14px; line-height:20px; }
.sidebar-heading strong { font-size:12px; font-weight:600; }
.asset-menu { display:flex; flex-direction:column; gap:6px; }
.asset-group { position:relative; display:flex; align-items:center; gap:10px; box-sizing:border-box; width:100%; min-height:60px; padding:8px 12px; border:1px solid transparent; border-radius:8px; background:transparent; color:var(--ops-text); cursor:pointer; text-align:left; font:inherit; transition:background .15s,border-color .15s; }
.asset-icon { display:grid; place-items:center; flex:0 0 34px; height:36px; border:1px solid var(--ops-border-soft); border-radius:8px; background:var(--ops-surface-soft); color:var(--ops-muted); font-size:18px; }
.repository-icon { color:var(--ops-warning); }
.asset-copy { display:flex; flex:1; min-width:0; flex-direction:column; gap:4px; }
.asset-copy strong { font-size:13px; font-weight:500; line-height:18px; }
.asset-copy small { color:var(--ops-subtle); font-size:10px; line-height:15px; white-space:nowrap; }
.asset-group:hover { background:var(--ops-surface-soft); }
.asset-group.active { background:color-mix(in srgb,var(--ops-primary) 6%,var(--ops-surface)); border-color:color-mix(in srgb,var(--ops-primary) 18%,var(--ops-surface)); }
.asset-group.active .asset-copy strong { color:var(--ops-primary); }
.asset-group.active .asset-icon { background:var(--ops-surface); color:var(--ops-primary); border-color:color-mix(in srgb,var(--ops-primary) 15%,var(--ops-surface)); }
.asset-group:focus-visible { outline:2px solid var(--ops-primary); outline-offset:1px; }
.asset-content { flex:1; min-width:0; min-height:0; overflow:auto; }
.asset-list { padding:16px; border:1px solid var(--ops-border); border-radius:8px; background:var(--ops-surface); }
.script-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.script-toolbar-actions, .row-actions, .drawer-footer { display: flex; align-items: center; gap: 8px; }.script-toolbar-actions :deep(.el-button + .el-button), .row-actions :deep(.el-button) { margin-left: 0; }
.script-search { width: 200px; }
.table-pagination { display: flex; justify-content: flex-end; padding-top: 16px; border-top: 1px solid #edf1f5; }
.runtime-label { color: #3f526d; font-family: 'SF Mono', Monaco, Consolas, monospace; font-size: 12px; }
.sync-state { display:flex; min-width:0; flex-direction:column; align-items:flex-start; gap:4px; }.sync-state small { max-width:100%; overflow:hidden; color:#8793a5; font-size:10px; text-overflow:ellipsis; white-space:nowrap; }
.drawer-title { display: flex; flex-direction: column; gap: 3px; color: var(--ink); font-size: 17px; font-weight: 700; }.drawer-title small { color: var(--muted); font-size: 12px; font-weight: 400; }
:deep(.el-select) { width: 100%; }
.drawer-footer { justify-content: flex-end; }
:global(.repository-drawer.el-drawer) { border-radius: 0 !important; }
@media (max-width: 680px) { .script-toolbar { align-items: stretch; flex-direction: column; }.script-search { width: 100%; }.script-toolbar-actions { justify-content: flex-end; } }
@media (max-width: 720px) {
  .page-container,.script-workbench { height:auto; }
  .script-workbench { flex-direction:column; }
  .asset-sidebar { width:100%; box-sizing:border-box; }
  .asset-menu { flex-direction:row; }
  .asset-group { flex:1; min-width:0; }
  .asset-content { overflow:visible; }
  .table-pagination { overflow-x:auto; justify-content:flex-start; }
}
@media (max-width: 420px) { .asset-group { padding:10px 8px; gap:8px; }.asset-copy small { white-space:normal; } }
@media (prefers-reduced-motion: reduce) { .asset-group { transition:none; } }
</style>

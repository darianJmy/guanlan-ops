<template>
  <div class="page-container automation-task-page">
    <header class="task-toolbar">
      <el-input v-model="keyword" clearable :prefix-icon="Search" aria-label="搜索自动化任务" placeholder="搜索任务名称" />
      <div><el-button :icon="Refresh" :loading="loading" @click="loadAll">刷新</el-button><el-button type="primary" :icon="Plus" @click="openEditor()">创建</el-button></div>
    </header>

    <el-card class="record-card" shadow="never">
      <el-table v-loading="loading" :data="filteredTasks" row-key="id" stripe>
        <el-table-column prop="name" label="任务名称" min-width="180" show-overflow-tooltip />
        <el-table-column label="运行环境" width="130"><template #default>标准脚本</template></el-table-column>
        <el-table-column label="脚本" min-width="170" show-overflow-tooltip><template #default="{ row }">{{ scriptText(row) }}</template></el-table-column>
        <el-table-column label="执行节点" min-width="160" show-overflow-tooltip><template #default="{ row }">{{ nodeText(row) }}</template></el-table-column>
        <el-table-column label="执行超时" width="120"><template #default="{ row }">{{ row.timeout_seconds }} 秒</template></el-table-column>
        <el-table-column label="状态" width="96"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">{{ row.status === 1 ? '启用' : '禁用' }}</el-tag></template></el-table-column>
        <el-table-column label="操作" width="190" fixed="right"><template #default="{ row }"><el-button link type="primary" :disabled="row.status !== 1" @click="openRun(row)">执行</el-button><el-button link type="primary" @click="openEditor(row)">编辑</el-button><el-button link type="danger" @click="removeTask(row)">删除</el-button></template></el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
    </el-card>

    <el-drawer v-model="editorVisible" class="standard-config-drawer automation-task-drawer" :size="choosingSource ? 'min(320px, 94vw)' : 'min(460px, 94vw)'" :close-on-click-modal="choosingSource" :close-on-press-escape="!saving" :show-close="!saving">
      <template #header><div class="drawer-header"><DrawerBackButton v-if="!choosingSource" :disabled="saving" @click="handleEditorBack" /><span v-if="choosingSource || form.id">{{ choosingSource ? '选择代码来源' : '编辑自动化任务' }}</span></div></template>
      <div v-if="choosingSource" class="source-cards" aria-label="选择代码来源">
        <button v-for="source in sourceOptions" :key="source.value" type="button" class="source-card" :aria-label="`使用${source.label}创建任务`" @click="selectSource(source.value)">
          <span class="source-icon"><el-icon><component :is="source.icon" /></el-icon></span>
          <strong>{{ source.label }}</strong><small>{{ source.description }}</small>
        </button>
      </div>
      <el-form v-else label-position="top" :disabled="saving">
        <div v-if="!form.id" class="selected-source"><span class="source-icon"><el-icon><component :is="selectedSource.icon" /></el-icon></span><strong>{{ selectedSource.label }}</strong></div>
        <el-form-item label="任务名称" required><el-input v-model="form.name" maxlength="128" /></el-form-item>
        <el-form-item v-if="form.id" label="代码来源" required class="top-gap"><el-radio-group v-model="form.source_type" @change="handleSourceTypeChange"><el-radio-button value="script">在线脚本</el-radio-button><el-radio-button value="repository">Git 仓库</el-radio-button></el-radio-group></el-form-item>
        <el-form-item v-if="form.source_type === 'script'" label="脚本" required><el-select v-model="form.script_id" filterable placeholder="请选择脚本"><el-option v-for="item in availableScripts" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item>
        <template v-else>
          <el-form-item label="Git 仓库" required><el-select v-model="form.repository_id" filterable placeholder="请选择已同步仓库"><el-option v-for="item in enabledRepositories" :key="item.id" :label="item.synced_revision ? item.name : `${item.name}（未同步）`" :value="item.id" :disabled="!item.synced_revision" /></el-select></el-form-item>
          <el-form-item label="执行命令" required><el-input v-model="form.command" type="textarea" :rows="4" class="mono-input" placeholder="例如 bash deploy.sh" /></el-form-item>
          <el-alert type="info" :closable="false" show-icon title="代码由脚本库手动同步；任务仅将本地代码部署到所选节点并执行。" />
        </template>
        <el-form-item label="执行节点" required><el-select v-model="form.node_key" filterable placeholder="请选择执行节点"><el-option label="平台本机" value="direct-0" /><el-option v-for="item in nodes" :key="item.key" :label="item.label" :value="item.key" /></el-select></el-form-item>
        <el-form-item label="超时时间"><el-input-number v-model="form.timeout_seconds" :min="1" :max="3600" controls-position="right" /><span class="unit">秒</span></el-form-item>
        <el-form-item label="备注"><el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" /></el-form-item>
        <el-form-item label="启用状态"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer><el-button :disabled="saving" @click="editorVisible = false">取消</el-button><el-button v-if="!choosingSource" type="primary" :loading="saving" @click="saveTask">保存</el-button></template>
    </el-drawer>

    <el-dialog v-model="runVisible" :title="`执行任务 · ${currentTask?.name || ''}`" width="620px">
      <div class="run-summary"><span>执行节点</span><strong>{{ currentTask ? nodeText(currentTask) : '—' }}</strong><span>执行脚本</span><strong>{{ currentTask ? scriptText(currentTask) : '—' }}</strong></div>
      <template>
        <div class="standard-run-copy"><strong>确认执行这个任务？</strong><span>手动执行默认不传入业务参数。</span></div>
        <el-button link type="primary" class="advanced-toggle" @click="advancedInputVisible = !advancedInputVisible">{{ advancedInputVisible ? '收起高级运行参数' : '高级运行参数' }}</el-button>
        <el-collapse-transition><div v-if="advancedInputVisible" class="advanced-input"><span>JSON 参数（可选）</span><el-input v-model="runInput" type="textarea" :rows="8" class="mono-input" placeholder="仅在脚本明确需要手动输入时填写" /><small>脚本可通过 AIOPS_INPUT_FILE 读取。</small></div></el-collapse-transition>
      </template>
      <div v-if="runResultVisible" class="run-result-wrap">
        <div class="run-result-title" :class="{ 'is-error': runResultFailed }">执行输出</div>
        <pre class="run-result">{{ runResult }}</pre>
      </div>
      <template #footer><el-button @click="runVisible = false">关闭</el-button><el-button type="primary" :loading="running" @click="executeTask">执行</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { Document, FolderOpened, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createAutomationTask, deleteAutomationTask, getAutomationTasks, runAutomationTask, updateAutomationTask } from '../../api/automationTask'
import { getScripts } from '../../api/script'
import { getAgents, getCertificateExecutors } from '../../api/certificate'
import { getGitRepositories } from '../../api/gitRepository'
import { agentTypeText } from '../../utils/agentNode'

const sourceOptions = [
  { value: 'script', label: '在线脚本', description: 'Shell · Python', icon: Document },
  { value: 'repository', label: 'Git 仓库', description: '仓库代码与执行命令', icon: FolderOpened },
]
const choosingSource = ref(false)
const emptyForm = () => ({ id: null, name: '', description: '', source_type:'script', script_id: null, repository_id:null, command:'', node_key: 'direct-0', timeout_seconds: 300, enabled: true })
const loading = ref(false), saving = ref(false), running = ref(false), editorVisible = ref(false), runVisible = ref(false)
const keyword = ref(''), tasks = ref([]), scripts = ref([]), repositories = ref([]), executors = ref([]), agents = ref([])
const form = reactive(emptyForm()), currentTask = ref(null), runInput = ref('{}'), runResult = ref(''), runResultVisible = ref(false), runResultFailed = ref(false), advancedInputVisible = ref(false)
const selectedSource = computed(() => sourceOptions.find(source => source.value === form.source_type) || sourceOptions[0])
const availableScripts = computed(() => scripts.value.filter(item => item.status === 1 && (item.source_type || 'inline') === 'inline'))
const enabledRepositories = computed(() => repositories.value.filter(item => item.status === 1))
const nodes = computed(() => [...executors.value.filter(item => item.status === 1).map(item => ({ key: `ssh-${item.id}`, label: `SSH · ${item.name}` })), ...agents.value.filter(item => item.status === 'online').map(item => ({ key: `agent-${item.id}`, label: `${agentTypeText(item)} · ${item.name}` }))])
const filteredTasks = computed(() => { const value = keyword.value.trim().toLowerCase(); return value ? tasks.value.filter(item => (item.name || '').toLowerCase().includes(value)) : tasks.value })

onMounted(loadAll)
async function loadAll() {
  loading.value = true
  try {
    const [taskResult, scriptResult, executorResult, agentResult, repositoryResult] = await Promise.all([
      getAutomationTasks(),
      getScripts(),
      getCertificateExecutors(),
      getAgents(),
      getGitRepositories(),
    ])
    tasks.value = taskResult.data || []
    scripts.value = scriptResult.data || []
    executors.value = executorResult.data || []
    agents.value = agentResult.data || []
    repositories.value = repositoryResult.data || []
  } finally {
    loading.value = false
  }
}
function openEditor(row) { choosingSource.value = !row; Object.assign(form, emptyForm(), row ? { ...row, node_key: `${row.node_type}-${row.node_id}`, enabled: row.status === 1 } : {}); editorVisible.value=true }
function handleSourceTypeChange() { form.script_id=null; form.repository_id=null; form.command='' }
function selectSource(type) {
  if (form.source_type !== type) {
    form.source_type = type
    handleSourceTypeChange()
  }
  choosingSource.value = false
}
function handleEditorBack() {
  if (saving.value) return
  if (form.id) editorVisible.value = false
  else choosingSource.value = true
}
function parseNode() { const [node_type, id] = String(form.node_key || '').split('-'); return { node_type, node_id: Number(id || 0) } }
async function saveTask() { if (choosingSource.value || saving.value) return; if (!form.name.trim()) return ElMessage.warning('请输入任务名称'); if (form.source_type==='script' && !form.script_id) return ElMessage.warning('请选择脚本'); if(form.source_type==='repository' && !form.repository_id) return ElMessage.warning('请选择已同步的 Git 仓库'); if(form.source_type==='repository' && !form.command.trim()) return ElMessage.warning('请输入执行命令'); if (!form.node_key) return ElMessage.warning('请选择执行节点'); const node=parseNode(); saving.value=true; try { const data={ name:form.name.trim(), description:form.description.trim(), source_type:form.source_type, script_id:Number(form.script_id||0), repository_id:Number(form.repository_id||0), command:form.command.trim(), ...node, timeout_seconds:Number(form.timeout_seconds), status:form.enabled?1:0 }; if(form.id) await updateAutomationTask(form.id,data); else await createAutomationTask(data); editorVisible.value=false; await loadAll(); ElMessage.success('自动化任务已保存') } finally { saving.value=false } }
async function removeTask(row) { await ElMessageBox.confirm(`确认删除自动化任务“${row.name}”？`, '删除任务', { type:'warning' }); await deleteAutomationTask(row.id); await loadAll(); ElMessage.success('自动化任务已删除') }
function openRun(row) { currentTask.value=row; runInput.value='{}'; advancedInputVisible.value=false; runResult.value=''; runResultVisible.value=false; runResultFailed.value=false; runVisible.value=true }
async function executeTask() { let input={}; if(advancedInputVisible.value) { try { input=JSON.parse(runInput.value); if(!input || Array.isArray(input) || typeof input!=='object') return ElMessage.warning('运行参数必须是 JSON 对象') } catch { return ElMessage.warning('请输入合法的 JSON 对象') } } running.value=true; runResultVisible.value=false; try { const response=await runAutomationTask(currentTask.value.id,input); const result=response.data||{}; runResultFailed.value=result.status!=='success'; runResult.value=result.output||result.error||'（无输出）'; runResultVisible.value=true; if(runResultFailed.value) ElMessage.error(`执行失败（退出码 ${result.exit_code}）`); else ElMessage.success('执行成功') } catch(error) { runResultFailed.value=true; runResult.value=error?.message||'执行请求失败'; runResultVisible.value=true } finally { running.value=false } }
function scriptText(row) { if(row.source_type==='repository') { const repository=repositories.value.find(item=>item.id===row.repository_id); return repository ? `${repository.name} · ${row.command}` : `Git 仓库 #${row.repository_id}` } return scripts.value.find(item=>item.id===row.script_id)?.name || `#${row.script_id}` }
function nodeText(row) { if(row.node_type==='direct') return '平台本机'; if(row.node_type==='ssh') return executors.value.find(item=>item.id===row.node_id)?.name || `SSH #${row.node_id}`; const agent=agents.value.find(item=>item.id===row.node_id); return agent ? `${agentTypeText(agent)} · ${agent.name}` : `Agent #${row.node_id}` }
</script>

<style scoped>
.automation-task-page { display: flex; min-width: 0; flex-direction: column; gap: 16px; }
.record-card { min-width: 0; border-color: var(--ops-border); border-radius: 10px; box-shadow: 0 8px 28px rgba(31, 50, 81, .045); }
.record-card :deep(.el-card__body) { padding: 16px; }
.task-toolbar > div :deep(.el-button + .el-button) { margin-left: 0; }
@media (max-width: 1100px) {
  .automation-task-page .task-toolbar { align-items: stretch; flex-direction: column; }
  .task-toolbar > div { justify-content: flex-end; }
}
@media (max-width: 680px) {
  .automation-task-page .task-toolbar > .el-input { width: 100%; }
}
.task-toolbar{display:flex;align-items:center;justify-content:space-between;gap:8px}.task-toolbar>.el-input{width:180px}.task-toolbar>div{display:flex;gap:8px}.drawer-header{display:flex;align-items:center;gap:12px;font-weight:600}.drawer-header .el-button{padding-left:0}.el-select{width:100%}.top-gap{margin-top:20px}.unit{margin-left:8px;color:var(--ops-muted)}.run-summary{display:grid;grid-template-columns:80px minmax(0,1fr);gap:8px 14px;margin-bottom:22px;padding:14px 16px;background:#f6f8fb;border:1px solid #e5eaf2;border-radius:6px}.run-summary span{color:var(--ops-muted);font-size:12px}.run-summary strong{min-width:0;overflow:hidden;color:var(--ops-text);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.run-context-tip{padding:10px 12px;color:#697386;background:#f8fafc;border-left:3px solid var(--el-color-primary);font-size:12px;line-height:1.6}.standard-run-copy{display:flex;flex-direction:column;gap:5px;color:var(--ops-text)}.standard-run-copy span{color:var(--ops-muted);font-size:12px}.advanced-toggle{margin:10px 0 4px;padding-left:0}.advanced-input{display:flex;flex-direction:column;gap:7px;padding:12px 14px;background:#f8fafc;border:1px solid #e5eaf2;border-radius:6px}.advanced-input>span{color:var(--ops-muted);font-size:12px;font-weight:600}.advanced-input small{color:var(--ops-muted)}.run-result,.mono-input :deep(textarea){font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.run-result-wrap{margin-top:16px}.run-result-title{margin-bottom:6px;color:var(--el-color-success);font-size:12px;font-weight:600}.run-result-title.is-error{color:var(--el-color-danger)}.run-result{max-height:260px;margin:0;overflow:auto;padding:12px;background:#111827;color:#d1fae5;border-radius:4px;white-space:pre-wrap}.run-result-title.is-error+.run-result{color:#fecaca}
:global(.automation-task-drawer.el-drawer){border-radius:0!important}
.source-cards { display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:12px; }
.source-card { display:flex; flex-direction:column; align-items:center; justify-content:center; gap:8px; min-height:124px; padding:16px 8px; border:1px solid var(--ops-border); border-radius:var(--ops-radius); background:var(--ops-surface); color:var(--ops-text); font:inherit; cursor:pointer; }
.source-icon { display:grid; place-items:center; width:36px; height:36px; border-radius:8px; background:var(--ops-surface-soft); color:var(--ops-primary); font-size:22px; }
.source-card strong { font-size:13px; font-weight:500; }
.source-card small { color:var(--ops-muted); font-size:10px; }
.source-card:hover { border-color:var(--ops-primary); background:var(--ops-primary-soft); }
.source-card:focus-visible { outline:2px solid var(--ops-primary); outline-offset:2px; }
.selected-source { display:flex; align-items:center; gap:10px; margin-bottom:20px; color:var(--ops-text); font-size:13px; }
.selected-source strong { font-weight:500; }
:global(.automation-task-drawer .el-drawer__header) { box-sizing:border-box; display:flex; align-items:center; height:64px; flex:0 0 64px; margin-bottom:0; padding:0 20px; border-bottom:1px solid var(--ops-border-soft); }
</style>

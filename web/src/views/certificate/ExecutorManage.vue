<template>
  <div class="settings-typography node-settings">
    <section class="node-list" v-loading="nodeLoading" aria-label="节点列表">
      <button v-if="!editorVisible" class="add-node-row" type="button" v-permission="['certificate:executor:edit']" @click="openEditor()"><el-icon><Plus /></el-icon><span>添加节点</span></button>
      <NodeEditor v-if="editorVisible && !form.id" :ref="setEditorRef" :form="form" :rules="rules" :node-types="nodeTypes" :saving="saving" @save="saveNode" @cancel="closeEditor" @select-type="selectType" />
      <template v-for="row in nodes" :key="row.node_key">
        <article class="node-row" :class="{ 'is-active': isEditing(row) }">
          <button class="node-summary" type="button" :disabled="saving" :aria-label="`查看节点 ${row.name}`" :aria-expanded="isEditing(row)" :aria-controls="`node-editor-${row.node_key}`" @click="toggleEditor(row)">
            <span class="node-mark" aria-hidden="true"><el-icon><component :is="row.node_type === 'ssh' ? Connection : row.node_type === 'docker' ? Platform : Monitor" /></el-icon></span>
            <span class="node-main"><strong>{{ row.name }}</strong></span>
          </button>
          <span class="node-status" :class="{ 'is-online': isAgentNode(row) ? row.status === 'online' : row.status === 1 }"><i />{{ nodeStatusText(row) }}</span>
          <el-icon class="row-arrow" aria-hidden="true"><ArrowDown /></el-icon>
        </article>
        <div v-if="isEditing(row)" :id="`node-editor-${row.node_key}`">
          <div v-if="isAgentNode(row)" class="node-details"><span>最近在线：{{ formatRelativeTime(row.last_seen_at) }}</span><span>版本：{{ row.version || '-' }}</span></div>
          <NodeEditor :ref="setEditorRef" :form="form" :rules="rules" :node-types="nodeTypes" :saving="saving" @save="saveNode" @cancel="closeEditor" @select-type="selectType">
            <template #actions>
              <el-button v-permission="['certificate:executor:edit']" type="danger" plain @click="removeNode(row)">删除</el-button>
              <el-button v-if="row.node_type === 'ssh'" :loading="testingId === row.id" @click="testSSH(row)">测试连接</el-button>
              <template v-else>
                <el-button v-permission="['certificate:executor:edit']" :loading="runningAgentId === row.id" @click="openAgentRunDialog(row)">执行命令</el-button>
                <el-button v-permission="['certificate:executor:edit']" @click="rotateToken(row)">刷新 Token</el-button>
              </template>
            </template>
          </NodeEditor>
        </div>
      </template>
    </section>

    <el-dialog v-model="tokenDialogVisible" :title="deploymentCommandTitle" width="760px">
      <div class="token-tip">Token 只显示这一次，部署后可在列表里看到 Agent 在线状态。</div>
      <div class="command-card">
        <el-button class="copy-command-button" size="small" :icon="CopyDocument" @click="copyAgentRunCommand">复制</el-button>
        <pre class="command-box" tabindex="0">{{ agentRunCommand }}</pre>
      </div>
      <template #footer>
        <el-button type="primary" @click="tokenDialogVisible = false">我知道了</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="agentCommandVisible" :title="`执行命令 - ${selectedAgent?.name || ''}`" width="760px" :close-on-click-modal="false">
      <el-input v-model="agentCommand" type="textarea" :rows="6" class="mono-input" />
      <pre v-if="agentCommandResult" class="command-result">{{ agentCommandResult }}</pre>
      <template #footer>
        <el-button @click="agentCommandVisible = false">关闭</el-button>
        <el-button type="primary" :loading="runningAgentId === selectedAgent?.id" @click="runSelectedAgentCommand">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import NodeEditor from '../../components/NodeEditor.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Connection, CopyDocument, Monitor, Platform, Plus } from '@element-plus/icons-vue'
import {
  createAgent,
  createCertificateExecutor,
  deleteAgent,
  deleteCertificateExecutor,
  getAgents,
  getCertificateExecutors,
  rotateAgentToken,
  runAgentCommand,
  testCertificateExecutor,
  updateAgent,
  updateCertificateExecutor,
} from '../../api/certificate'
import { agentDeploymentType, agentDockerImage } from '../../utils/agentNode'

const executors = ref([])
const agents = ref([])
const loading = ref(false)
const loadingAgents = ref(false)
const saving = ref(false)
const testingId = ref(null)
const editorVisible = ref(false)
const tokenDialogVisible = ref(false)
const agentCommandVisible = ref(false)
const formRef = ref()
const agentRunCommand = ref('')
const deploymentCommandTitle = ref('Agent 部署命令')
const selectedAgent = ref(null)
const agentCommand = ref('echo agent-ok && hostname')
const agentCommandResult = ref('')
const runningAgentId = ref(null)
let agentPollTimer = null

const nodeTypes = [
  { value: 'ssh', label: 'SSH 节点', description: '平台主动连接主机执行任务', icon: Connection },
  { value: 'agent', label: 'Agent 节点', description: 'Agent 主动连接平台执行任务', icon: Monitor },
  { value: 'docker', label: 'Docker 节点', description: 'Agent 在独立容器环境中执行任务', icon: Platform },
]

const form = reactive({
  id: null,
  type: 'ssh',
  name: '',
  host: '',
  port: 22,
  username: '',
  auth_type: 'password',
  password: '',
  private_key: '',
  enabled: true,
  region: '',
  docker_image: 'aiops-agent:latest',
  description: '',
})

const rules = {
  name: [{ required: true, message: '请输入节点名称', trigger: 'blur' }],
  host: [{ required: true, message: '请输入主机地址', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
}

const nodes = computed(() => [
  ...executors.value.map(item => ({ ...item, node_type: 'ssh', node_key: `ssh-${item.id}` })),
  ...agents.value.map(item => ({ ...item, node_type: agentDeploymentType(item), execution_type: 'agent', node_key: `agent-${item.id}` })),
])
const nodeLoading = computed(() => loading.value || loadingAgents.value)

onMounted(() => {
  loadAll()
  startAgentPolling()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onBeforeUnmount(() => {
  stopAgentPolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})

async function loadAll() {
  await Promise.all([loadExecutors(), loadAgents()])
}

async function loadExecutors() {
  loading.value = true
  try {
    const res = await getCertificateExecutors()
    executors.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function loadAgents(options = {}) {
  const silent = Boolean(options.silent)
  if (!silent) loadingAgents.value = true
  try {
    const res = await getAgents()
    agents.value = res.data || []
  } finally {
    if (!silent) loadingAgents.value = false
  }
}

function startAgentPolling() {
  stopAgentPolling()
  agentPollTimer = window.setInterval(() => {
    if (document.hidden) return
    loadAgents({ silent: true })
  }, 10000)
}

function stopAgentPolling() {
  if (!agentPollTimer) return
  window.clearInterval(agentPollTimer)
  agentPollTimer = null
}

function handleVisibilityChange() {
  if (!document.hidden) loadAgents({ silent: true })
}

function setEditorRef(value) { formRef.value = value }
function isEditing(row) { return editorVisible.value && form.id === row.id && form.type === row.node_type }
function closeEditor() { if (saving.value) return; editorVisible.value = false; form.password = ''; form.private_key = '' }
function toggleEditor(row) { if (saving.value) return; if (isEditing(row)) closeEditor(); else openEditor(row) }
function openEditor(row) {
  if (saving.value) return
  if (row && isAgentNode(row)) {
    resetForm({
      id: row.id,
      type: row.node_type,
      name: row.name,
      region: row.region || '',
      docker_image: agentDockerImage(row) || 'aiops-agent:latest',
      description: row.description || '',
    })
  } else if (row) {
    resetForm({
      id: row.id,
      type: 'ssh',
      name: row.name,
      host: row.host,
      port: row.port || 22,
      username: row.username,
      auth_type: row.auth_type || 'password',
      enabled: row.status === 1,
      description: row.description || '',
    })
  } else {
    resetForm()
  }
  editorVisible.value = true
}

function resetForm(values = {}) {
  Object.assign(form, {
    id: null,
    type: 'ssh',
    name: '',
    host: '',
    port: 22,
    username: '',
    auth_type: 'password',
    password: '',
    private_key: '',
    enabled: true,
    region: '',
    docker_image: 'aiops-agent:latest',
    description: '',
  }, values)
  formRef.value?.clearValidate?.()
}

function selectType(type) {
  if (form.id || form.type === type) return
  resetForm({ type })
}

async function saveNode() {
  if (saving.value) return
  saving.value = true
  try {
    if (form.type === 'ssh') await saveSSHNode()
    else await saveAgentNode()
  } finally {
    saving.value = false
    if (!editorVisible.value) {
      form.password = ''
      form.private_key = ''
    }
  }
}

async function saveSSHNode() {
  const payload = {
    name: form.name,
    type: 'ssh',
    host: form.host,
    port: form.port,
    username: form.username,
    auth_type: form.auth_type,
    password: form.password,
    private_key: form.private_key,
    status: form.enabled ? 1 : 2,
    description: form.description,
  }
  if (form.id) await updateCertificateExecutor(form.id, payload)
  else await createCertificateExecutor(payload)
  ElMessage.success('SSH 节点已保存')
  editorVisible.value = false
  await loadExecutors()
}

async function saveAgentNode() {
  const payload = {
    name: form.name,
    region: form.region,
    description: form.description,
    deployment_type: form.type,
    docker_image: form.type === 'docker' ? form.docker_image.trim() : '',
  }
  if (form.id) {
    await updateAgent(form.id, payload)
    ElMessage.success(`${form.type === 'docker' ? 'Docker' : 'Agent'} 节点已保存`)
  } else {
    const res = await createAgent(payload)
    if (form.type === 'docker') {
      deploymentCommandTitle.value = 'Docker 节点部署命令'
      agentRunCommand.value = buildDockerAgentCommand(res.data.token, form.region, form.docker_image, form.name, res.data.agent?.id)
    } else {
      deploymentCommandTitle.value = 'Agent 部署命令'
      agentRunCommand.value = buildAgentCommand(res.data.token, form.region)
    }
    tokenDialogVisible.value = true
  }
  editorVisible.value = false
  await loadAgents()
}

async function testSSH(row) {
  testingId.value = row.id
  try {
    await testCertificateExecutor(row.id)
    ElMessage.success('连接成功')
  } finally {
    testingId.value = null
  }
}

async function removeNode(row) {
  const typeName = nodeTypeText(row)
  await ElMessageBox.confirm(`确认删除 ${typeName} 节点 ${row.name}？`, '删除确认', { type: 'warning' })
  if (isAgentNode(row)) {
    await deleteAgent(row.id)
    await loadAgents()
  } else {
    await deleteCertificateExecutor(row.id)
    await loadExecutors()
  }
  if (isEditing(row)) closeEditor()
  ElMessage.success('已删除')
}

async function rotateToken(row) {
  await ElMessageBox.confirm(`确认刷新 ${nodeTypeText(row)} ${row.name} 的 Token？旧 Token 会立即失效。`, '刷新 Token', { type: 'warning' })
  const res = await rotateAgentToken(row.id)
  if (row.node_type === 'docker') {
    deploymentCommandTitle.value = 'Docker 节点部署命令'
    agentRunCommand.value = buildDockerAgentCommand(res.data.token, row.region, agentDockerImage(row), row.name, row.id)
  } else {
    deploymentCommandTitle.value = 'Agent 部署命令'
    agentRunCommand.value = buildAgentCommand(res.data.token, row.region)
  }
  tokenDialogVisible.value = true
}

function openAgentRunDialog(row) {
  selectedAgent.value = row
  agentCommand.value = 'echo agent-ok && hostname'
  agentCommandResult.value = ''
  agentCommandVisible.value = true
}

async function runSelectedAgentCommand() {
  if (!selectedAgent.value) return
  if (!agentCommand.value.trim()) {
    ElMessage.warning('请输入执行命令')
    return
  }
  runningAgentId.value = selectedAgent.value.id
  try {
    const res = await runAgentCommand(selectedAgent.value.id, {
      command: agentCommand.value,
      timeout_seconds: 300,
    })
    agentCommandResult.value = formatAgentCommandResult(res.data)
    ElMessage.success('执行完成')
  } finally {
    runningAgentId.value = null
  }
}

function formatAgentCommandResult(result = {}) {
  const parts = [
    `exit_code: ${result.exit_code ?? 0}`,
    `duration: ${result.duration_ms ?? 0}ms`,
  ]
  if (result.stdout) parts.push(`\n[stdout]\n${String(result.stdout).trimEnd()}`)
  if (result.stderr) parts.push(`\n[stderr]\n${String(result.stderr).trimEnd()}`)
  if (result.error) parts.push(`\n[error]\n${result.error}`)
  return parts.join('\n')
}

function buildAgentCommand(token, region) {
  const installURL = new URL('/api/agent/install.sh', window.location.origin)
  installURL.searchParams.set('token', token)
  if (region) installURL.searchParams.set('region', region)
  return `curl -fsSL ${shellQuote(installURL.toString())} | sh`
}

function buildDockerAgentCommand(token, region, image, name, id) {
  const websocketURL = new URL('/api/agent/connect', window.location.origin)
  websocketURL.protocol = websocketURL.protocol === 'https:' ? 'wss:' : 'ws:'
  const dockerImage = String(image || '').trim() || 'aiops-agent:latest'
  const containerName = dockerContainerName(name, id)
  return [
    'docker run -d \\',
    `  --name ${shellQuote(containerName)} \\`,
    '  --restart unless-stopped \\',
    '  --init \\',
    '  --read-only \\',
    '  --tmpfs /tmp:rw,exec,size=512m \\',
    `  -e AIOPS_SERVER=${shellQuote(websocketURL.toString())} \\`,
    `  -e AIOPS_AGENT_TOKEN=${shellQuote(token)} \\`,
    `  -e AIOPS_AGENT_REGION=${shellQuote(region)} \\`,
    `  -e AIOPS_AGENT_IMAGE=${shellQuote(dockerImage)} \\`,
    `  ${shellQuote(dockerImage)}`,
  ].join('\n')
}

function dockerContainerName(name, id) {
  const normalized = String(name || '').toLowerCase().replace(/[^a-z0-9_.-]+/g, '-').replace(/^[_.-]+|[_.-]+$/g, '')
  return `aiops-agent-${normalized || id || 'docker'}`.slice(0, 120)
}

function isAgentNode(row) {
  return row?.node_type === 'agent' || row?.node_type === 'docker'
}

function nodeTypeText(row) {
  if (row?.node_type === 'docker') return 'Docker'
  if (row?.node_type === 'agent') return 'Agent'
  return 'SSH'
}

async function copyAgentRunCommand() {
  const command = agentRunCommand.value
  if (!command) return
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(command)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = command
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const copied = document.execCommand('copy')
      textarea.remove()
      if (!copied) throw new Error('copy failed')
    }
    ElMessage.success('部署命令已复制')
  } catch {
    ElMessage.error('复制失败，请手动选择命令复制')
  }
}

function shellQuote(value) {
  return `'${String(value || '').replace(/'/g, `'\\''`)}'`
}

function nodeAddress(row) {
  if (row.node_type === 'ssh') return `${row.host || '-'}:${row.port || 22}`
  if (row.hostname && row.remote_addr) return `${row.hostname} (${row.remote_addr})`
  return row.hostname || row.remote_addr || '-'
}

function nodeConnection(row) {
  if (row.node_type === 'docker') return agentDockerImage(row) || 'aiops-agent:latest'
  if (row.node_type === 'agent') return row.region || '-'
  const auth = row.auth_type === 'private_key' ? '私钥' : '密码'
  return `${row.username || '-'} / ${auth}`
}

function nodeStatusText(row) {
  if (isAgentNode(row)) return row.status === 'online' ? '在线' : '离线'
  return row.status === 1 ? '启用' : '停用'
}

function formatRelativeTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const seconds = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000))
  if (seconds < 60) return '刚刚'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} 分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} 小时前`
  return `${Math.floor(hours / 24)} 天前`
}
</script>

<style scoped>
.node-settings{max-width:980px;margin:0 auto}.node-list{min-height:160px}
.add-node-row{display:flex;width:100%;min-height:58px;align-items:center;gap:12px;margin:0 0 12px;padding:0 18px;color:var(--ops-primary);background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 55%,var(--ops-border));cursor:pointer;font:inherit;font-size:13px;font-weight:600;text-align:left}.add-node-row .el-icon{font-size:21px}.add-node-row:hover{background:color-mix(in srgb,var(--ops-primary) 4%,var(--ops-surface));border-color:var(--ops-primary)}
.node-row{position:relative;display:grid;grid-template-columns:minmax(0,1fr) auto 16px;align-items:center;gap:14px;min-height:70px;margin-bottom:8px;padding:10px 16px;border:1px solid var(--ops-border);background:var(--ops-surface)}.node-row:hover,.node-row.is-active{border-color:color-mix(in srgb,var(--ops-primary) 55%,var(--ops-border));box-shadow:0 2px 9px color-mix(in srgb,var(--ops-text) 7%,transparent)}
.node-summary{display:flex;align-items:center;gap:16px;min-width:0;padding:0;border:0;background:transparent;color:inherit;text-align:left;font:inherit;cursor:pointer}.node-summary::after{position:absolute;inset:0;content:''}.node-summary:disabled{cursor:wait}.node-summary:focus-visible,.add-node-row:focus-visible{outline:2px solid var(--ops-primary);outline-offset:2px}
.node-mark{display:grid;place-items:center;flex:0 0 36px;width:36px;height:36px;color:var(--ops-text);background:var(--ops-surface-soft);border:1px solid var(--ops-border-soft);border-radius:6px;font-size:20px}.node-main{display:flex;flex:1;min-width:0;align-items:center;gap:20px}.node-main strong{font-weight:400;flex:0 1 180px;min-width:0;overflow:hidden;color:var(--ops-text);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.node-main>span{flex:1;min-width:0;overflow:hidden;color:var(--ops-muted);font-size:11px;text-overflow:ellipsis;white-space:nowrap}
.node-status{display:flex;align-items:center;gap:5px;color:var(--ops-muted);font-size:11px;white-space:nowrap}.node-status i{width:6px;height:6px;border-radius:50%;background:var(--ops-muted)}.node-status.is-online{color:var(--el-color-success)}.node-status.is-online i{background:var(--el-color-success)}.row-arrow{color:var(--ops-muted);pointer-events:none;transition:transform .18s}.node-row.is-active .row-arrow{transform:rotate(180deg)}
.node-details{display:flex;flex-wrap:wrap;gap:16px;margin:-8px 0 8px;padding:12px 18px;color:var(--ops-muted);font-size:11px;background:var(--ops-surface-soft);border:1px solid var(--ops-primary);border-top:0;border-bottom:0}
.mono-input :deep(textarea) { font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.45; }
.token-tip { margin-bottom: 10px; color: var(--ops-muted); font-size: 13px; }
.command-card { position: relative; }
.copy-command-button { position: absolute; z-index: 1; top: 10px; right: 10px; color: #dbeafe; background: #253349; border-color: #3b4b63; }
.copy-command-button:hover,
.copy-command-button:focus-visible { color: #fff; background: #34517a; border-color: #5f82b2; }
.command-box { margin: 0; padding: 48px 14px 14px; max-height: 280px; overflow-y: auto; color: #d1d5db; background: #111827; border-radius: 6px; font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-all; user-select: text; }
.command-result { margin: 12px 0 0; padding: 14px; max-height: 360px; overflow: auto; color: #d1d5db; background: #111827; border-radius: 6px; font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.6; white-space: pre-wrap; }


.node-main>.node-description{flex:0 1 160px}
@media(max-width:760px){.node-main>.node-description{display:none}}
@media(max-width:760px){.node-main{gap:10px}.node-main strong{flex-basis:40%}}
</style>

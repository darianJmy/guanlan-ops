<template>
  <div class="page-container">
    <div class="node-toolbar">
      <el-input v-model="nodeSearchInput" clearable aria-label="搜索节点" placeholder="搜索名称、地址或备注" class="node-search" @keyup.enter="searchNodes" @clear="searchNodes">
        <template #append><el-button aria-label="搜索" @click="searchNodes"><el-icon><Search /></el-icon></el-button></template>
      </el-input>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" :loading="nodeLoading" @click="loadAll">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openDrawer()" v-permission="['certificate:executor:edit']">
          创建
        </el-button>
      </div>
    </div>

    <section class="node-panel">
      <el-table :data="filteredNodes" v-loading="nodeLoading" row-key="node_key" class="node-table" border>
        <el-table-column prop="name" label="节点名称" min-width="160" show-overflow-tooltip />
        <el-table-column label="节点类型" width="110" align="center">
          <template #default="{ row }">
            <el-tag :type="nodeTypeTagType(row)" size="small" effect="plain">
              {{ nodeTypeText(row) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="节点地址" min-width="210" show-overflow-tooltip>
          <template #default="{ row }">{{ nodeAddress(row) }}</template>
        </el-table-column>
        <el-table-column label="连接信息" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">{{ nodeConnection(row) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="nodeStatusType(row)" size="small">{{ nodeStatusText(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最近在线" width="150">
          <template #default="{ row }">{{ isAgentNode(row) ? formatRelativeTime(row.last_seen_at) : '-' }}</template>
        </el-table-column>
        <el-table-column label="版本" width="90" align="center">
          <template #default="{ row }">{{ isAgentNode(row) ? (row.version || '-') : '-' }}</template>
        </el-table-column>
        <el-table-column prop="description" label="备注" min-width="150" show-overflow-tooltip>
          <template #default="{ row }">{{ row.description || '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="330" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <template v-if="row.node_type === 'ssh'">
                <el-button link type="primary" :loading="testingId === row.id" @click="testSSH(row)">测试</el-button>
              </template>
              <template v-else>
                <el-button link type="primary" :loading="runningAgentId === row.id" @click="openAgentRunDialog(row)" v-permission="['certificate:executor:edit']">执行命令</el-button>
                <el-button link type="primary" @click="rotateToken(row)" v-permission="['certificate:executor:edit']">刷新 Token</el-button>
              </template>
              <el-button link type="primary" @click="openDrawer(row)" v-permission="['certificate:executor:edit']">编辑</el-button>
              <el-button link type="danger" @click="removeNode(row)" v-permission="['certificate:executor:edit']">删除</el-button>
            </div>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
    </section>

    <el-drawer v-model="drawerVisible" class="node-drawer" size="min(460px, 94vw)" direction="rtl" :close-on-click-modal="false">
      <template #header>
        <div class="drawer-header">
          <DrawerBackButton @click="drawerVisible = false" />
          <span class="drawer-title">{{ form.id ? '编辑节点' : '创建节点' }}</span>
        </div>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="drawer-form">
        <el-form-item label="节点类型" prop="type">
          <div class="type-card-list">
            <button
              v-for="item in nodeTypes"
              :key="item.value"
              class="type-card"
              :class="{ active: form.type === item.value, disabled: form.id && form.type !== item.value }"
              type="button"
              :disabled="!!form.id"
              @click="selectType(item.value)"
            >
              <span class="type-icon"><el-icon><component :is="item.icon" /></el-icon></span>
              <span class="type-content">
                <strong>{{ item.label }}</strong>
                <small>{{ item.description }}</small>
              </span>
            </button>
          </div>
        </el-form-item>

        <el-form-item label="节点名称" prop="name">
          <el-input v-model="form.name" :placeholder="form.type === 'ssh' ? 'production-web-01' : (form.type === 'docker' ? 'python-data-container' : 'oversea-sg-01')" />
        </el-form-item>

        <template v-if="form.type === 'ssh'">
          <div class="form-grid">
            <el-form-item label="主机地址" prop="host">
              <el-input v-model="form.host" placeholder="10.0.0.1" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" />
            </el-form-item>
          </div>
          <el-form-item label="用户名" prop="username">
            <el-input v-model="form.username" placeholder="root" />
          </el-form-item>
          <el-form-item label="认证方式">
            <el-radio-group v-model="form.auth_type">
              <el-radio-button value="password">密码</el-radio-button>
              <el-radio-button value="private_key">私钥</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="form.auth_type === 'password'" label="密码">
            <el-input v-model="form.password" type="password" show-password autocomplete="off" :placeholder="form.id ? '不修改请留空' : ''" />
          </el-form-item>
          <el-form-item v-else label="私钥">
            <el-input v-model="form.private_key" type="textarea" :rows="8" class="mono-input" :placeholder="form.id ? '不修改请留空' : '-----BEGIN OPENSSH PRIVATE KEY-----'" />
          </el-form-item>
          <el-form-item label="状态">
            <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
          </el-form-item>
        </template>

        <template v-else>
          <el-form-item label="地域">
            <el-input v-model="form.region" placeholder="新加坡 / 美国 / 香港" />
          </el-form-item>
          <el-form-item v-if="form.type === 'docker'" label="Docker 镜像">
            <el-input v-model="form.docker_image" :disabled="!!form.id" placeholder="aiops-agent:latest" />
          </el-form-item>
          <el-alert
            v-if="!form.id"
            :title="form.type === 'docker' ? '创建后直接生成 docker run 命令，请在已安装 Docker 的主机执行。' : '创建后会生成一次性 Token 和部署命令，请及时复制并在目标主机执行。'"
            type="info"
            :closable="false"
            show-icon
          />
        </template>

        <el-form-item label="备注" class="description-field">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
      </el-form>

      <template #footer>
        <div class="drawer-footer">
          <el-button @click="drawerVisible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="saveNode">
            {{ form.type !== 'ssh' && !form.id ? '创建并生成' : '保存' }}
          </el-button>
        </div>
      </template>
    </el-drawer>

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
      <el-input v-model="agentCommand" type="textarea" :rows="6" class="mono-input" placeholder="输入要在 Agent 机器上执行的 Shell 命令" />
      <pre v-if="agentCommandResult" class="command-result">{{ agentCommandResult }}</pre>
      <template #footer>
        <el-button @click="agentCommandVisible = false">关闭</el-button>
        <el-button type="primary" :loading="runningAgentId === selectedAgent?.id" @click="runSelectedAgentCommand">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import DrawerBackButton from '../../components/DrawerBackButton.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowLeft, Connection, CopyDocument, Monitor, Platform, Plus, Refresh, Search } from '@element-plus/icons-vue'
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
const drawerVisible = ref(false)
const tokenDialogVisible = ref(false)
const agentCommandVisible = ref(false)
const formRef = ref()
const agentRunCommand = ref('')
const deploymentCommandTitle = ref('Agent 部署命令')
const selectedAgent = ref(null)
const agentCommand = ref('echo agent-ok && hostname')
const agentCommandResult = ref('')
const runningAgentId = ref(null)
const nodeSearchInput = ref('')
const nodeSearchKeyword = ref('')
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
const filteredNodes = computed(() => {
  const keyword = nodeSearchKeyword.value.toLowerCase()
  if (!keyword) return nodes.value
  return nodes.value.filter(item => [
    item.name,
    item.node_type,
    item.host,
    item.username,
    item.region,
    item.description,
    nodeAddress(item),
    nodeConnection(item),
  ].some(value => String(value || '').toLowerCase().includes(keyword)))
})
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

function searchNodes() {
  nodeSearchKeyword.value = nodeSearchInput.value.trim()
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

function openDrawer(row) {
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
  drawerVisible.value = true
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
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    if (form.type === 'ssh') await saveSSHNode()
    else await saveAgentNode()
  } finally {
    saving.value = false
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
  drawerVisible.value = false
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
  drawerVisible.value = false
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

function nodeTypeTagType(row) {
  if (row?.node_type === 'docker') return 'warning'
  if (row?.node_type === 'agent') return 'success'
  return 'primary'
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

function nodeStatusType(row) {
  if (isAgentNode(row)) return row.status === 'online' ? 'success' : 'info'
  return row.status === 1 ? 'success' : 'info'
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
.page-container { display: flex; flex-direction: column; gap: 12px; }
.node-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.node-search { width: 240px; }
.toolbar-actions { display: flex; align-items: center; gap: 10px; }
.node-panel { background: var(--ops-surface); border: 1px solid #e5e8ef; border-radius: 6px; padding: 14px; }
:deep(.node-table.el-table) { --el-table-border-color: #edf0f5; --el-table-header-bg-color: #f7f9fc; }
:deep(.node-table .el-table__header-wrapper th) { color: #5d6675; font-weight: 700; }
:deep(.node-table .el-table__cell) { padding: 11px 0; }
:deep(.node-table .el-table__row:hover > td.el-table__cell) { background: #f8fbff; }
.row-actions { display: flex; align-items: center; gap: 10px; white-space: nowrap; }
.row-actions :deep(.el-button) { margin-left: 0; }
:deep(.node-drawer.el-drawer) { border-radius: 0; }
:deep(.node-drawer .el-drawer__header) { margin-bottom: 0; padding: 18px 20px; border-bottom: 1px solid #e5e9f2; }
:deep(.node-drawer .el-drawer__body) { background: var(--ops-surface); padding: 18px 20px; }
:deep(.node-drawer .el-drawer__footer) { border-top: 1px solid #e5e9f2; padding: 12px 20px; }
.drawer-header { display: flex; align-items: center; justify-content: flex-start; gap: 12px; width: 100%; }
.back-button { height: 32px; padding: 0 8px; font-size: 14px; color: var(--ops-primary); }
.drawer-title { color: var(--ops-text); font-size: 16px; font-weight: 600; }
.drawer-form :deep(.el-form-item) { margin-bottom: 18px; }
.type-card-list { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; width: 100%; }
.type-card { display: flex; align-items: center; gap: 12px; min-height: 76px; padding: 14px; text-align: left; color: #334155; background: var(--ops-surface); border: 1px solid #dfe5ed; border-radius: 8px; cursor: pointer; transition: border-color .2s, box-shadow .2s, background .2s; }
.type-card:hover { border-color: #9fc1ec; background: #f8fbff; }
.type-card.active { border-color: var(--ops-primary); background: #f3f8ff; box-shadow: 0 0 0 1px var(--ops-primary) inset; }
.type-card.disabled { cursor: not-allowed; opacity: .5; }
.type-icon { display: inline-flex; align-items: center; justify-content: center; width: 40px; height: 40px; flex: 0 0 40px; color: #3b82f6; background: #eaf3ff; border-radius: 8px; font-size: 22px; }
.type-content { display: flex; flex-direction: column; gap: 3px; }
.type-content strong { color: var(--ops-text); font-size: 14px; }
.type-content small { color: #8490a2; font-size: 12px; line-height: 1.4; }
.form-grid { display: grid; grid-template-columns: minmax(0, 1fr) 150px; gap: 14px; }
.form-grid :deep(.el-input-number) { width: 100%; }
.description-field { margin-top: 18px; }
.drawer-footer { display: flex; justify-content: flex-end; gap: 8px; width: 100%; }
.mono-input :deep(textarea) { font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.45; }
.token-tip { margin-bottom: 10px; color: var(--ops-muted); font-size: 13px; }
.command-card { position: relative; }
.copy-command-button { position: absolute; z-index: 1; top: 10px; right: 10px; color: #dbeafe; background: #253349; border-color: #3b4b63; }
.copy-command-button:hover,
.copy-command-button:focus-visible { color: #fff; background: #34517a; border-color: #5f82b2; }
.command-box { margin: 0; padding: 48px 14px 14px; max-height: 280px; overflow-y: auto; color: #d1d5db; background: #111827; border-radius: 6px; font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.6; white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-all; user-select: text; }
.command-result { margin: 12px 0 0; padding: 14px; max-height: 360px; overflow: auto; color: #d1d5db; background: #111827; border-radius: 6px; font-family: 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace; font-size: 12px; line-height: 1.6; white-space: pre-wrap; }
@media (max-width: 760px) {
  .node-toolbar { align-items: stretch; flex-direction: column; }
  .node-search { width: 100%; }
  .toolbar-actions { width: 100%; justify-content: flex-end; }
  .type-card-list,
  .form-grid { grid-template-columns: 1fr; }
}

.form-grid { grid-template-columns:minmax(0,1fr); }
</style>

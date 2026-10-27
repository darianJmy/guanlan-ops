<template>
  <div class="editor-page" v-loading="loading">
    <div class="preview-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="cancel">返回</el-button>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="editor-layout">
      <div class="editor-main">
        <div class="section-title"><strong>基本信息</strong></div>
        <div class="form-grid">
          <el-form-item label="任务名称" prop="name"><el-input v-model="form.name" maxlength="100" show-word-limit /></el-form-item>
          <el-form-item label="描述"><el-input v-model="form.description" maxlength="500" show-word-limit /></el-form-item>
        </div>
        <el-form-item class="external-label-field" label="额外标签">
          <div class="label-editor">
            <el-button class="add-label-button" :icon="Plus" plain @click="addExternalLabel">添加标签</el-button>
            <div v-for="(item, index) in form.external_labels" :key="`label-${index}`" class="label-row">
              <el-input v-model="item.key" placeholder="Key" maxlength="64" />
              <el-input v-model="item.value" placeholder="Value" maxlength="200" />
              <el-button class="label-icon-button" :icon="Minus" circle plain aria-label="删除标签" @click="form.external_labels.splice(index, 1)" />
            </div>
          </div>
        </el-form-item>

        <div class="section-title"><strong>规则配置</strong><el-tooltip :content="currentType.hint" placement="top"><el-icon class="section-help-icon" tabindex="0" aria-label="规则配置说明"><QuestionFilled /></el-icon></el-tooltip></div>
        <el-form-item label="拨测类型">
          <ProbeTypeSelect :model-value="form.type" @change="changeType" />
        </el-form-item>
        <div v-if="form.type === 'http'" class="form-grid">
          <el-form-item class="full-row method-field" label="请求方式"><el-radio-group v-model="form.method"><el-radio-button v-for="method in httpMethods" :key="method" :value="method">{{ method }}</el-radio-button></el-radio-group></el-form-item>
          <el-form-item class="full-row" label="请求 URL" prop="target"><el-input v-model="form.target" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" /></el-form-item>
          <el-form-item v-if="requestBodyVisible" class="full-row" label="请求 Body"><el-input v-model="form.body" type="textarea" :rows="4" class="mono-input" placeholder='{"key":"value"}' /></el-form-item>
          <el-form-item class="full-row" label="请求 Headers">
            <key-value-editor v-model="form.headers" key-placeholder="" value-placeholder="" />
          </el-form-item>
          <el-form-item class="full-row" label="查询 Params">
            <key-value-editor v-model="form.params" key-placeholder="" value-placeholder="" />
          </el-form-item>
        </div>
        <div v-else-if="form.type === 'script'" class="form-grid">
          <el-form-item class="full-row" label="执行脚本" prop="script_id">
            <el-select v-model="form.script_id" filterable placeholder="选择脚本库中的脚本" aria-label="执行脚本">
              <el-option v-for="script in scripts" :key="script.id" :label="`${script.name} · ${script.type === 'python' ? 'Python' : 'Shell'}${script.status !== 1 ? '（已停用）' : ''}`" :value="script.id" :disabled="script.status !== 1" />
            </el-select>
          </el-form-item>
          <div class="script-hint full-row" role="note"><el-icon aria-hidden="true"><InfoFilled /></el-icon><span>每次执行使用脚本库的最新内容。退出码 0 表示成功，非 0、超时或执行异常表示失败；执行输出保存在拨测记录中。</span></div>
        </div>
        <div v-else class="form-grid">
          <el-form-item :label="form.type === 'certificate_expiry' ? '网站域名' : '目标 Host'" prop="host"><el-input v-model="form.host" /></el-form-item>
          <el-form-item v-if="form.type === 'tcp'" label="端口"><el-input-number v-model="form.port" :min="1" :max="65535" /></el-form-item>
          <el-form-item v-if="form.type === 'certificate_expiry'" label="提前多少天告警"><div class="interval-control"><el-input-number v-model="form.expiry_alert_days" :min="1" :max="365" :precision="0" controls-position="right" aria-label="提前告警天数" /><span>天</span></div></el-form-item>
          <el-form-item v-if="form.type === 'icmp'" label="发包数量"><el-input-number v-model="form.count" :min="1" :max="10" /></el-form-item>
        </div>
        <el-collapse class="advanced-collapse full-row">
          <el-collapse-item name="advanced">
            <template #title>
              <span class="advanced-title">高级设置</span>
            </template>
            <div class="form-grid advanced-grid">
              <el-form-item label="超时时间（ms）" :class="{ 'full-row': form.type !== 'http' }">
                <el-input-number v-model="form.timeout_ms" :min="100" :max="300000" :step="1000" controls-position="right" />
              </el-form-item>
              <el-form-item v-if="form.type === 'certificate_expiry'" class="full-row" label="SNI / Server Name"><el-input v-model="form.server_name" placeholder="留空时使用网站域名" /></el-form-item>
              <template v-if="form.type === 'http'">
                <el-form-item label="Content-Type">
                  <el-select v-model="form.content_type" filterable allow-create default-first-option>
                    <el-option label="application/json" value="application/json" />
                    <el-option label="application/x-www-form-urlencoded" value="application/x-www-form-urlencoded" />
                    <el-option label="text/plain" value="text/plain" />
                  </el-select>
                </el-form-item>
                <el-form-item class="full-row auth-field" label="认证方式">
                  <el-radio-group v-model="form.auth_type">
                    <el-radio-button value="none">无认证</el-radio-button>
                    <el-radio-button value="basic">Basic Auth</el-radio-button>
                  </el-radio-group>
                </el-form-item>
                <template v-if="form.auth_type === 'basic'">
                  <el-form-item label="用户名" prop="username"><el-input v-model="form.username" /></el-form-item>
                  <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
                </template>
                <el-form-item label="成功状态码"><el-input v-model="form.success_codes" placeholder="200-399 或 200,201,204" /></el-form-item>
                <el-form-item label="响应关键字（包含）"><el-input v-model="form.response_keyword" clearable placeholder="可选" /></el-form-item>
              </template>

            </div>
          </el-collapse-item>
        </el-collapse>

        <div class="form-grid condition-node-grid">
          <el-form-item label="告警条件">
          <el-select v-model="form.severity" aria-label="告警条件"><el-option label="P0" value="P0" /><el-option label="P1" value="P1" /><el-option label="P2" value="P2" /></el-select>
        </el-form-item>
          <el-form-item label="执行节点">
            <el-select v-model="selectedNode" aria-label="执行节点" placeholder="请选择执行节点"><el-option v-if="legacyNodeValues.length > 1" :label="`保留原有 ${legacyNodeValues.length} 个节点`" value="__legacy__" /><el-option label="服务端直连" value="direct" /><el-option-group v-if="sshNodes.length" label="SSH 节点"><el-option v-for="node in sshNodes" :key="`ssh:${node.id}`" :label="node.name" :value="`ssh:${node.id}`" :disabled="node.status !== 1" /></el-option-group><el-option-group v-if="hostAgentNodes.length" label="Agent 节点"><el-option v-for="node in hostAgentNodes" :key="`agent:${node.id}`" :label="node.name" :value="`agent:${node.id}`" :disabled="node.status !== 'online'" /></el-option-group><el-option-group v-if="dockerAgentNodes.length" label="Docker 节点"><el-option v-for="node in dockerAgentNodes" :key="`agent:${node.id}`" :label="node.name" :value="`agent:${node.id}`" :disabled="node.status !== 'online'" /></el-option-group></el-select>
            <p v-if="legacyNodeValues.length > 1" class="legacy-node-note">原任务包含多个执行节点；选择单个节点并保存后，将替换原配置。</p>
          </el-form-item>
        </div>

        <el-form-item class="alert-detail-field">
          <template #label><span class="detail-label">告警详情<el-tooltip content="支持变量：任务信息使用 ${labels.task_name}、${labels.target}、${labels.probe_node}，额外标签使用 ${external_labels.env}。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="告警详情变量说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-input v-model="form.alert_detail" type="textarea" :rows="4" maxlength="2000" show-word-limit placeholder="例如：${external_labels.env} 环境的拨测任务 ${labels.task_name} 在 ${labels.probe_node} 节点检测到 ${labels.target} 异常" />
        </el-form-item>

        <div class="section-title"><strong>调度与告警</strong></div>
        <div class="schedule-policy-list">
          <div class="schedule-policy-row">
            <div class="schedule-policy-copy"><span class="detail-label"><strong>多久拨测一次</strong><el-tooltip content="最低 10 秒" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="拨测频率说明"><QuestionFilled /></el-icon></el-tooltip></span></div>
            <div v-if="form.type === 'certificate_expiry'" class="schedule-policy-control"><div class="interval-control"><el-input-number v-model="intervalHours" :min="1" :max="24" :precision="0" :controls="false" aria-label="检查周期（小时）" /><span>小时</span></div></div>
            <div v-else class="schedule-policy-control"><div class="interval-control"><el-input-number v-model="form.interval_seconds" :min="10" :max="86400" :precision="0" :controls="false" aria-label="拨测周期（秒）" /><span>秒</span></div></div>
          </div>
          <div class="schedule-policy-row">
            <div class="schedule-policy-copy"><strong>连续失败</strong></div>
            <div class="schedule-policy-control"><div class="interval-control"><el-input-number v-model="form.failure_threshold" :min="1" :max="100" :controls="false" aria-label="连续失败次数" /><span>次</span></div></div>
          </div>
          <div class="schedule-policy-row">
            <div class="schedule-policy-copy"><strong>恢复判定</strong></div>
            <div class="schedule-policy-control"><div class="interval-control"><el-input-number v-model="form.recovery_threshold" :min="1" :max="100" :controls="false" aria-label="连续成功次数" /><span>次</span></div></div>
          </div>
          <div class="schedule-policy-row">
            <div class="schedule-policy-copy"><strong>故障中心</strong></div>
            <div class="schedule-policy-control"><el-form-item class="schedule-policy-form-item" prop="fault_center_id"><el-select v-model="form.fault_center_id" filterable aria-label="故障中心" placeholder="请选择故障中心"><el-option v-for="center in activeCenters" :key="center.id" :label="center.name" :value="center.id" /></el-select></el-form-item></div>
          </div>
        </div>
        <div class="status-line"><strong>启用规则</strong><el-switch v-model="form.status" :active-value="1" :inactive-value="0" /></div>
      </div>

      <editor-action-bar>
        <template #leading>
          <el-button type="primary" plain :loading="testing" @click="handleTest" v-permission="['inspection:run']">测试拨测</el-button>
          <span class="editor-action-hint">测试拨测不会保存任务，也不会触发通知</span>
        </template>
        <el-button @click="cancel">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSubmit">{{ isEdit ? '保存修改' : '创建任务' }}</el-button>
      </editor-action-bar>
    </el-form>

    <el-dialog v-model="testVisible" title="测试拨测结果" width="min(640px, 92vw)" append-to-body>
      <div v-if="testResult" class="test-result">
        <div class="test-result-state" :class="testResult.status === 'success' ? 'success' : 'failed'">
          <strong>{{ testResult.status === 'success' ? '拨测通过' : '拨测失败' }}</strong>
          <span>耗时 {{ testResult.duration_ms || 0 }}ms</span>
        </div>
        <div v-if="form.type === 'certificate_expiry'" class="expiry-tests"><div v-for="(entry, index) in testResult.result" :key="index"><p>{{ entry.node }}</p><CertificateExpiryResult :result="entry.result" /></div></div>
        <div v-else-if="form.type === 'script'"><div v-for="(entry, index) in testResult.result" :key="index"><p>{{ entry.node }} · {{ entry.duration_ms }}ms</p><ScriptProbeResult :result="entry.result" /></div></div>
        <pre v-else>{{ formatTestResult(testResult) }}</pre>
      </div>
      <template #footer><el-button @click="testVisible = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElButton, ElInput, ElMessage } from 'element-plus'
import { ArrowLeft, Delete, InfoFilled, Minus, Plus, QuestionFilled } from '@element-plus/icons-vue'
import { getAllInspectionRuleGroups, createInspectionItem, getInspectionItems, updateInspectionItem } from '../../api/inspection'
import { getAgents, getCertificateExecutors } from '../../api/certificate'
import { getScripts } from '../../api/script'
import ScriptProbeResult from '../../components/ScriptProbeResult.vue'
import { getFaultCenters } from '../../api/alert'
import { isDockerAgent } from '../../utils/agentNode'
import { instantProbe } from '../../api/probe'
import CertificateExpiryResult from '../../components/CertificateExpiryResult.vue'
import ProbeTypeSelect from '../../components/ProbeTypeSelect.vue'
import { probeTypes } from '../../utils/probeTypes'
import { parseCertificateTarget, formatCertificateTarget } from '../../utils/certificateTarget'
import EditorActionBar from '../../components/EditorActionBar.vue'

const KeyValueEditor = defineComponent({ name: 'KeyValueEditor', props: { modelValue: { type: Array, required: true }, keyPlaceholder: { type: String, default: '键' }, valuePlaceholder: { type: String, default: '值' } }, emits: ['update:modelValue'], setup(props, { emit }) { const update = (index, field, value) => emit('update:modelValue', props.modelValue.map((row, i) => i === index ? { ...row, [field]: value } : row)); const add = () => emit('update:modelValue', [...props.modelValue, { key: '', value: '' }]); const remove = index => { const rows = props.modelValue.filter((_, i) => i !== index); emit('update:modelValue', rows.length ? rows : [{ key: '', value: '' }]) }; return () => h('div', { class: 'kv-editor' }, props.modelValue.map((row, index) => h('div', { class: 'kv-row', key: index }, [h(ElInput, { modelValue: row.key, placeholder: props.keyPlaceholder, 'onUpdate:modelValue': value => update(index, 'key', value) }), h(ElInput, { modelValue: row.value, placeholder: props.valuePlaceholder, 'onUpdate:modelValue': value => update(index, 'value', value) }), h(ElButton, { icon: index === props.modelValue.length - 1 ? Plus : Delete, circle: true, onClick: index === props.modelValue.length - 1 ? add : () => remove(index) })]))) } })
const taskTypes = probeTypes

const defaultForm = (type = 'http') => ({ id: null, group_id: null, name: '', description: '', type, script_id: null, target: '', host: '', port: taskTypes.find(item => item.value === type)?.defaultPort || 80, method: 'GET', headers: [{ key: '', value: '' }], params: [{ key: '', value: '' }], body: '', timeout_ms: type === 'script' ? 30000 : 10000, content_type: 'application/json', auth_type: 'none', username: '', password: '', success_codes: '200-399', response_keyword: '', count: 4, server_name: '', node_values: ['direct'], expiry_alert_days: 30, interval_seconds: type === 'certificate_expiry' ? 21600 : 300, failure_threshold: type === 'certificate_expiry' ? 1 : 3, recovery_threshold: type === 'certificate_expiry' ? 1 : 3, alert_detail: '', severity: 'P2', fault_center_id: null, external_labels: [], status: 1 })
const route = useRoute(), router = useRouter(), formRef = ref(), form = reactive(defaultForm())
const loading = ref(true), saving = ref(false), testing = ref(false), testVisible = ref(false), testResult = ref(null)
const ruleGroups = ref([]), scripts = ref([])
const legacyNodeValues = ref([])
const selectedNode = computed({
  get: () => form.node_values.length > 1 ? '__legacy__' : form.node_values[0] || '',
  set: value => { form.node_values = value === '__legacy__' ? [...legacyNodeValues.value] : value ? [value] : [] },
})
const sshNodes = ref([]), agentNodes = ref([]), faultCenters = ref([])
const hostAgentNodes = computed(() => agentNodes.value.filter(item => !isDockerAgent(item)))
const dockerAgentNodes = computed(() => agentNodes.value.filter(isDockerAgent))
const isEdit = computed(() => Boolean(route.params.id)), currentType = computed(() => taskTypes.find(item => item.value === form.type) || taskTypes[0]), requestBodyVisible = computed(() => ['POST', 'PUT'].includes(form.method))
const activeCenters = computed(() => faultCenters.value.filter(item => item.status === 1))
const targetSummary = computed(() => form.type === 'script' ? scripts.value.find(script => script.id === form.script_id)?.name || '尚未选择脚本' : form.type === 'http' ? form.target.trim() || '尚未配置' : `${form.host.trim() || '尚未配置'}${form.type !== 'icmp' && form.host.trim() ? `:${form.port}` : ''}`)
const nodeSummary = computed(() => form.node_values.map(nodeDisplayName).join('、') || '尚未选择')
const intervalHours = computed({ get: () => form.interval_seconds / 3600, set: value => { form.interval_seconds = value * 3600 } })
const intervalSummary = computed(() => form.type === 'certificate_expiry' ? `每 ${form.interval_seconds / 3600} 小时` : `每 ${form.interval_seconds} 秒`)
const faultCenterSummary = computed(() => activeCenters.value.find(item => item.id === form.fault_center_id)?.name || '尚未选择')
const recoverySummary = computed(() => `连续 ${form.recovery_threshold} 次成功后恢复`)
const labelPreview = computed(() => form.external_labels.filter(item => String(item.key || '').trim()))
const httpMethods = ['GET', 'POST', 'PUT', 'DELETE']
const rules = { script_id: [{ validator: (_r, value, done) => form.type !== 'script' || scripts.value.some(script => script.id === value && script.status === 1) ? done() : done(new Error('请选择启用的脚本')), trigger: 'change' }], name: [{ required: true, message: '请输入任务名称', trigger: 'blur' }], target: [{ validator: (_r, value, done) => form.type !== 'http' || /^https?:\/\/[^\s]+$/i.test(String(value || '').trim()) ? done() : done(new Error('请输入完整 URL')), trigger: 'blur' }], host: [{ validator: (_r, value, done) => !['icmp', 'tcp', 'certificate_expiry'].includes(form.type) || String(value || '').trim() ? done() : done(new Error('请输入目标 Host')), trigger: 'blur' }], fault_center_id: [{ required: true, message: '请选择故障中心', trigger: 'change' }], username: [{ validator: (_r, value, done) => form.auth_type === 'none' || String(value || '').trim() ? done() : done(new Error('请输入认证用户名')), trigger: 'blur' }] }
onMounted(loadData)
async function loadData() {
  try {
    const results = await Promise.allSettled([getInspectionItems(), getCertificateExecutors(), getAgents(), getFaultCenters(), getAllInspectionRuleGroups(), getScripts()])
    if (results[5].status === 'fulfilled') scripts.value = results[5].value.data || []
    if (results[4].status === 'fulfilled') ruleGroups.value = results[4].value
    if (!isEdit.value) form.group_id = ruleGroups.value.find(group => group.id === Number(route.query.group_id))?.id || null
    const items = results[0].status === 'fulfilled' ? results[0].value.data || [] : []
    if (results[1].status === 'fulfilled') sshNodes.value = results[1].value.data || []
    if (results[2].status === 'fulfilled') agentNodes.value = results[2].value.data || []
    if (results[3].status === 'fulfilled') faultCenters.value = results[3].value.data || []
    if (!isEdit.value && activeCenters.value.length === 1) form.fault_center_id = activeCenters.value[0].id
    if (isEdit.value || route.query.copy_from) {
      const row = items.find(item => item.id === Number(isEdit.value ? route.params.id : route.query.copy_from))
      if (!row) { ElMessage.error('拨测任务不存在'); return cancel() }
      if (isEdit.value) {
        fillForm(row)
        legacyNodeValues.value = form.node_values.length > 1 ? [...form.node_values] : []
      } else {
        copyTask(row, items)
      }
    }
  } finally { loading.value = false }
}
function copyTask(row, items) {
  fillForm(row)
  form.id = null
  const existingNames = new Set(items.filter(item => item.group_id === row.group_id).map(item => item.name))
  const base = [...row.name].slice(0, 110).join('')
  let name = `${base} 副本`, index = 2
  while (existingNames.has(name)) name = `${base} 副本 ${index++}`
  form.name = name
  if (form.node_values.length !== 1) form.node_values = []
  legacyNodeValues.value = []
}
function fillForm(row) { let config = {}; try { config = JSON.parse(row.config || '{}') } catch {} Object.assign(form, defaultForm(row.type), config, { id: row.id, group_id: row.group_id, name: row.name, type: row.type, interval_seconds: row.interval_seconds || 300, failure_threshold: row.failure_threshold || 3, recovery_threshold: row.recovery_threshold || 3, severity: row.severity || 'P2', fault_center_id: row.fault_center_id || null, external_labels: objectToLabelRows(parseObject(row.external_labels)), status: row.status, headers: objectToRows(config.headers), params: objectToRows(config.params), node_values: configuredNodeValues(config) }); if (row.type === 'http' && !form.target) form.target = row.target; if (['icmp', 'tcp', 'certificate_expiry'].includes(row.type) && !form.host) form.host = row.target; if (row.type === 'certificate_expiry') form.host = formatCertificateTarget(form.host, form.port) }
function addExternalLabel() { form.external_labels.push({ key: '', value: '' }) }
function changeType(type) { if (form.type === type) return; const nodes = form.node_values; Object.assign(form, { ...defaultForm(type), id: form.id, group_id: form.group_id, name: form.name, description: form.description, node_values: nodes, interval_seconds: type === 'certificate_expiry' ? 21600 : form.interval_seconds, failure_threshold: type === 'certificate_expiry' ? 1 : form.failure_threshold, recovery_threshold: type === 'certificate_expiry' ? 1 : form.recovery_threshold, alert_detail: form.alert_detail, severity: form.severity, fault_center_id: form.fault_center_id, external_labels: form.external_labels, status: form.status }); formRef.value?.clearValidate?.() }
function selectedExecutionNodes() { return form.node_values.map(value => { const [nodeType, rawNodeID] = value.includes(':') ? value.split(':') : ['direct', '0']; return { node_type: nodeType, node_id: Number(rawNodeID) || 0 } }) }
function buildPayload() { const common = { description: form.description.trim(), execution_nodes: selectedExecutionNodes(), timeout_ms: form.timeout_ms, alert_detail: form.alert_detail.trim() }; let config; if (form.type === 'http') config = { ...common, target: form.target.trim(), method: form.method, headers: rowsToObject(form.headers), params: rowsToObject(form.params), body: form.body, content_type: form.content_type, auth_type: form.auth_type, username: form.username, password: form.password, success_codes: form.success_codes, response_keyword: form.response_keyword.trim() }; else if (form.type === 'script') config = { ...common, script_id: form.script_id }; else config = { ...common, ...(form.type === 'certificate_expiry' ? parseCertificateTarget(form.host) : { host: form.host.trim(), port: form.port }), count: form.count, server_name: form.server_name.trim(), ...(form.type === 'certificate_expiry' ? { expiry_alert_days: form.expiry_alert_days } : {}) }; const target = form.type === 'script' ? scripts.value.find(script => script.id === form.script_id)?.name || '' : form.type === 'http' ? form.target.trim() : form.host.trim(); return { group_id: form.group_id, name: form.name.trim(), type: form.type, target, config: JSON.stringify(config), external_labels: JSON.stringify(rowsToObject(form.external_labels)), expected_result: '拨测成功', interval_seconds: form.interval_seconds, failure_threshold: form.failure_threshold, recovery_threshold: form.recovery_threshold, severity: form.severity, fault_center_id: form.fault_center_id || 0, status: form.status } }
function testConfigError() { if (form.type === 'certificate_expiry') { try { parseCertificateTarget(form.host) } catch { return '请输入有效的网站域名和端口（1 至 65535）' } } if (form.type === 'script' && !scripts.value.some(script => script.id === form.script_id && script.status === 1)) return '请选择启用的脚本'; if (form.type === 'certificate_expiry' && (!Number.isInteger(form.expiry_alert_days) || form.expiry_alert_days < 1 || form.expiry_alert_days > 365)) return '提前告警天数应为 1-365 天'; if (!form.node_values.length) return '请选择一个执行节点'; if (form.type === 'http' && !/^https?:\/\/[^\s]+$/i.test(form.target.trim())) return '请先填写完整的请求 URL'; if (['icmp', 'tcp', 'certificate_expiry'].includes(form.type) && !form.host.trim()) return '请先填写目标 Host'; return '' }
async function handleTest() { const error = testConfigError(); if (error) return ElMessage.warning(error); testing.value = true; try { const config = JSON.parse(buildPayload().config); const responses = await Promise.all(selectedExecutionNodes().map(async node => { const response = await instantProbe({ type: form.type, ...config, ...node }); return { node: nodeDisplayName(node.node_type === 'direct' ? 'direct' : `${node.node_type}:${node.node_id}`), ...response.data } })); testResult.value = { status: responses.every(item => item.status === 'success') ? 'success' : 'fail', duration_ms: Math.max(0, ...responses.map(item => item.duration_ms || 0)), result: responses }; testVisible.value = true } finally { testing.value = false } }
function formatTestResult(value) { const raw = value?.result ?? value; try { return JSON.stringify(typeof raw === 'string' ? JSON.parse(raw) : raw, null, 2) } catch { return String(raw || '(无输出)') } }
async function handleSubmit() { if (!form.group_id) return ElMessage.warning('请从拨测规则组进入后创建任务'); if (testConfigError()) return ElMessage.warning(testConfigError()); const labelError = validateLabels(form.external_labels); if (labelError) return ElMessage.warning(labelError); if (!form.node_values.length) return ElMessage.warning('请选择一个执行节点'); if (form.interval_seconds < 10 || form.interval_seconds > 86400) return ElMessage.warning('拨测周期应为 10 至 86400 秒'); if (!await formRef.value.validate().catch(() => false)) return; saving.value = true; try { const payload = buildPayload(); form.id ? await updateInspectionItem(form.id, payload) : await createInspectionItem(payload); ElMessage.success(form.id ? '任务已更新' : '任务已创建'); router.push(form.group_id ? `/inspection/items/groups/${form.group_id}` : '/inspection/items') } finally { saving.value = false } }
function rowsToObject(rows) { return rows.reduce((result, row) => { const key = String(row.key || '').trim(); if (key) result[key] = row.value || ''; return result }, {}) }
function objectToRows(value) { const rows = Object.entries(value || {}).map(([key, entry]) => ({ key, value: entry })); return rows.length ? rows : [{ key: '', value: '' }] }
function objectToLabelRows(value) { return Object.entries(value || {}).map(([key, entry]) => ({ key, value: entry })) }
function configuredNodeValues(config) { const nodes = Array.isArray(config.execution_nodes) ? config.execution_nodes : []; return [...new Set(nodes.map(node => node.node_type && node.node_type !== 'direct' ? `${node.node_type}:${node.node_id}` : 'direct'))] }
function nodeDisplayName(value) { if (value === 'direct') return '服务端直连'; const [type, rawID] = value.split(':'); const list = type === 'ssh' ? sshNodes.value : agentNodes.value; return list.find(item => item.id === Number(rawID))?.name || `${type.toUpperCase()} #${rawID}` }
function parseObject(value) { if (typeof value === 'object' && value) return value; try { return JSON.parse(value || '{}') } catch { return {} } }
function labelToneClass(key) { const normalized = String(key || '').trim().toLowerCase(); const preferred = { env: 'yellow', owner: 'purple', service: 'blue' }; if (preferred[normalized]) return `scope-tag-${preferred[normalized]}`; const tones = ['blue', 'yellow', 'purple', 'cyan', 'green']; const hash = [...normalized].reduce((total, character) => total + character.charCodeAt(0), 0); return `scope-tag-${tones[hash % tones.length]}` }
function validateLabels(rows) { const filled = rows.filter(item => String(item.key || '').trim() || String(item.value || '').trim()); if (filled.length > 20) return '任务标签最多 20 个'; const keys = new Set(); for (const row of filled) { const key = String(row.key || '').trim(); if (!key) return '请填写标签键'; if (!/^[A-Za-z0-9_.-]+$/.test(key)) return '标签键只能包含字母、数字、点、下划线和短横线'; const normalized = key.toLowerCase(); if (keys.has(normalized)) return `标签键 ${key} 重复`; keys.add(normalized) } return '' }
function cancel() { router.push(form.group_id ? `/inspection/items/groups/${form.group_id}` : '/inspection/items') }
</script>

<style scoped>
.editor-page{--ink:var(--ops-text);--muted:var(--ops-muted);--border:var(--ops-border);display:flex;min-height:calc(100vh - 100px);flex-direction:column;gap:16px}.editor-header{display:flex;align-items:center;gap:14px;padding:17px 20px;background:var(--ops-surface);border:1px solid var(--border);border-radius:10px}.editor-header>div{display:flex;flex:1;flex-direction:column}.editor-header h1{margin:0;color:var(--ink);font-size:20px}.editor-header p{margin:4px 0 0;color:var(--muted);font-size:12px}.draft-state{display:flex;align-items:center;gap:7px;color:var(--ops-muted);font-size:11px}.draft-state i{width:7px;height:7px;background:#e3a52f;border-radius:50%;box-shadow:0 0 0 4px rgba(227,165,47,.12)}.editor-layout{display:grid;grid-template-columns:minmax(0,1fr);gap:16px;align-items:start;padding-bottom:76px}.editor-main{min-width:0;padding:22px 24px;background:var(--ops-surface);border:1px solid var(--border);border-radius:8px}.section-title{display:flex;align-items:center;gap:10px;margin-bottom:20px;padding-bottom:14px;border-bottom:1px solid var(--ops-border-soft)}.section-title>div{display:flex;flex-direction:column}.section-title strong{color:#344054;font-size:14px;font-weight:600}.section-title small{margin-top:3px;color:#8a96a8;font-size:11px}.section-index{display:flex;width:28px;height:28px;align-items:center;justify-content:center;color:var(--ops-primary);background:#edf5ff;border-radius:7px;font-size:9px;font-weight:800}.field-label{display:block;margin-bottom:8px;color:#46536a;font-size:13px;font-weight:650}.probe-types{display:grid;grid-template-columns:repeat(5,minmax(0,1fr));gap:9px}.type-card{display:flex;min-width:0;flex-direction:column;gap:3px;padding:12px;color:#617087;text-align:left;background:#fafbfd;border:1px solid var(--ops-border);border-radius:8px;cursor:pointer;transition:.18s}.type-card:hover{border-color:#a9c8ee;transform:translateY(-1px)}.type-card.active{color:#1c5fb9;background:#f3f8ff;border-color:#6da5ea;box-shadow:0 5px 14px rgba(23,105,224,.08)}.type-card>span{font-size:10px;font-weight:800}.type-card strong{color:#28384f;font-size:12px;white-space:nowrap}.type-card small{overflow:hidden;color:var(--ops-muted);font-size:9px;text-overflow:ellipsis;white-space:nowrap}.form-grid,.schedule-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:0 20px}.schedule-grid{grid-template-columns:repeat(4,minmax(0,1fr))}.schedule-interval{grid-column:span 2}.full-row{grid-column:1/-1}:deep(.el-form-item){margin-bottom:18px}:deep(.el-form-item__label){padding-bottom:0;color:#46536a;font-size:12px;font-weight:400}:deep(.el-select),:deep(.el-input-number){width:100%}.method-field :deep(.el-radio-group){display:grid;grid-template-columns:repeat(4,minmax(0,1fr));width:100%}.method-field :deep(.el-radio-button),.method-field :deep(.el-radio-button__inner){width:100%}.field-unit{position:absolute;right:12px;color:#8a96a8;font-size:11px}.mono-input :deep(textarea){font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.advanced-collapse{overflow:hidden;background:#f8fafc;border:1px solid var(--border);border-radius:8px}.advanced-collapse :deep(.el-collapse-item__header){height:50px;padding:0 15px;background:#f8fafc;border:0}.advanced-collapse :deep(.el-collapse-item__wrap){border:0}.advanced-collapse :deep(.el-collapse-item__content){padding:16px 16px 0}.advanced-title{color:#315781;font-weight:700}.advanced-title+small{margin-left:8px;color:#8a96a8}.execution-panel{display:grid;grid-template-columns:minmax(0,1fr);gap:0;align-items:center;padding:16px 18px;background:#f8fafc;border:1px solid #dfe7f1;border-radius:8px}.execution-panel p{margin:0;color:#7d8b9f;font-size:11px;line-height:1.6}.status-field{display:flex;align-items:center;gap:8px;color:#69778a;font-size:11px}.editor-actions{position:fixed;right:20px;bottom:0;left:calc(var(--sidebar-width,220px) + 20px);display:flex;min-height:62px;align-items:center;justify-content:space-between;padding:0 22px;background:rgba(255,255,255,.96);border-top:1px solid #dfe6ee;box-shadow:0 -7px 22px rgba(31,50,81,.07);z-index:9}.editor-actions>span{color:#8a96a8;font-size:10px}.editor-actions>div{display:flex;gap:9px}:deep(.kv-editor){display:flex;width:100%;flex-direction:column;gap:8px}:deep(.kv-row){display:grid;grid-template-columns:1fr 1fr 34px;gap:9px}.option-meta{float:right;color:#96a0af}@media(prefers-reduced-motion:reduce){.type-card{transition:none}}@media(max-width:1050px){.editor-layout{grid-template-columns:1fr}.probe-types{grid-template-columns:repeat(3,1fr)}}@media(max-width:700px){.editor-header{align-items:flex-start}.draft-state{display:none}.probe-types,.form-grid,.schedule-grid,.execution-panel{grid-template-columns:1fr}.full-row,.schedule-interval{grid-column:auto}.editor-actions{left:0;padding:0 14px}.editor-actions>span{display:none}}
.section-help-icon{color:var(--ops-subtle);cursor:help;font-size:14px}.section-help-icon:hover,.section-help-icon:focus-visible{color:var(--ops-primary);outline:none}.label-editor{display:flex;width:100%;flex-direction:column;gap:8px}.add-label-button{width:100%;border-style:dashed}.label-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr) 28px;align-items:center;gap:8px}.label-icon-button{width:26px;height:26px;color:#8a94a3;background:var(--ops-surface);border-color:var(--ops-border)}.label-icon-button :deep(.el-icon){font-size:14px}.label-icon-button:hover,.label-icon-button:focus-visible{color:#606b7b;background:var(--ops-surface-soft);border-color:#c8cdd5}
.status-line{display:flex;align-items:center;justify-content:space-between;padding:15px 16px;border:1px solid #e6eaf0;border-radius:7px;background:var(--ops-surface-soft)}.status-line strong{color:#344054;font-size:13px}
.alert-detail-field{margin-top:2px}.detail-label{display:inline-flex;align-items:center;gap:5px}.detail-help-icon{color:var(--ops-subtle);cursor:help;font-size:14px}.detail-help-icon:hover,.detail-help-icon:focus-visible{color:var(--ops-primary);outline:none}.alert-detail-field :deep(.el-textarea__inner::placeholder){color:#a8abb2}
.schedule-grid :deep(.el-form-item__content){width:100%}.interval-control{display:grid;width:100%;grid-template-columns:minmax(0,1fr) 38px;align-items:stretch;overflow:hidden;border:1px solid var(--ops-border);border-radius:6px;background:var(--ops-surface);transition:border-color .2s,box-shadow .2s}.interval-control:focus-within{border-color:var(--ops-primary);box-shadow:0 0 0 2px rgba(64,158,255,.1)}.interval-control :deep(.el-input-number){width:100%}.interval-control :deep(.el-input__wrapper){box-shadow:none}.interval-control>span{display:flex;align-items:center;justify-content:center;color:var(--ops-subtle);background:var(--ops-surface-soft);border-left:1px solid var(--ops-border);font-size:12px;text-align:center}
.schedule-policy-list{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px;margin-bottom:18px}.schedule-policy-row{display:flex;min-width:0;flex-direction:column;gap:8px}.schedule-policy-copy{display:flex;min-width:0;flex-direction:column;gap:4px}.schedule-policy-copy strong{color:#344054;font-size:13px;font-weight:650}.schedule-policy-copy small{color:#8a96a8;font-size:11px;line-height:1.5}.schedule-policy-control{width:100%;margin-top:auto}
.schedule-policy-control :deep(.el-select){width:100%}.schedule-policy-form-item{margin-bottom:0}.schedule-policy-form-item :deep(.el-form-item__content){width:100%}
@media(max-width:700px){.schedule-policy-list{grid-template-columns:1fr}.schedule-policy-control{width:100%;justify-self:stretch}}

/* Keep task creation visually aligned with the instant probe workspace. */
.editor-page {
  --ink: var(--ops-text);
  --muted: var(--ops-muted);
  --border:var(--ops-border);
  --blue: var(--ops-primary);
  gap: 16px;
}
.preview-header { display: flex; align-items: center; justify-content: space-between; }


.execution-panel {
  border-color: #dfe7f1;
  border-radius: 8px;
}
.execution-panel { background: #f8fafc; }
.auth-field :deep(.el-radio-group) {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  width: 100%;
}
.auth-field :deep(.el-radio-button),
.auth-field :deep(.el-radio-button__inner) {
  width: 100%;
}
.scope-tags{display:flex;align-items:center;align-content:center;flex-wrap:wrap;gap:6px;padding-top:12px;border-top:1px solid var(--ops-border-soft)}
.scope-tag{display:inline-flex;min-width:0;max-width:100%;height:24px;align-items:center;padding:0 8px;overflow:hidden;border:1px solid transparent;border-radius:4px;font-size:12px;line-height:22px;text-overflow:ellipsis;white-space:nowrap}
.scope-tag-yellow{color:#ad6800;background:#fffbe6;border-color:#ffe58f}.scope-tag-purple{color:#531dab;background:#f9f0ff;border-color:#d3adf7}.scope-tag-blue{color:#1d39c4;background:#f0f5ff;border-color:#adc6ff}.scope-tag-cyan{color:#006d75;background:#e6fffb;border-color:#87e8de}.scope-tag-green{color:#237804;background:#f6ffed;border-color:#b7eb8f}
.test-result-state {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 11px 13px;
  border-left: 3px solid;
  background: #f7f9fc;
}
.test-result-state.success { color: #28745b; border-color: #21a179; background: #f2faf7; }
.test-result-state.failed { color: #a34848; border-color: #d95757; background: #fdf5f5; }
.test-result-state span { font-size: 12px; }
.test-result pre {
  max-height: 360px;
  margin: 12px 0 0;
  padding: 14px;
  overflow: auto;
  color: #d8e2ef;
  background: #111b2e;
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
@media (max-width: 1050px) {

}
.legacy-node-note{margin:8px 0 0;color:var(--ops-muted);font-size:12px;line-height:1.6}
.editor-main>.section-title:not(:first-child){margin-top:24px}
@media(max-width:700px){.editor-main{padding:18px 16px}}
.condition-node-grid{margin-top:20px}
.script-hint{display:flex;align-items:flex-start;gap:8px;margin:-4px 0 18px;padding:9px 11px;color:var(--ops-muted);background:color-mix(in srgb,var(--ops-primary) 6%,transparent);font-size:11px;line-height:1.55}
.script-hint .el-icon{margin-top:2px;flex:none;color:var(--ops-primary)}
</style>

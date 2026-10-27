<template>
  <div class="rule-editor-page" v-loading="loading">
    <div class="preview-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="cancel">返回</el-button>
    </div>

    <el-form ref="formRef" :model="form" :rules="formRules" label-position="top" class="editor-form">
      <div class="rule-config-card">
        <div class="section-heading"><div><strong>基础配置</strong></div><small>{{ group.name || '当前规则组' }}</small></div>
        <div class="two-column">
          <el-form-item label="规则名称" prop="name"><el-input v-model="form.name" maxlength="128" show-word-limit /></el-form-item>
          <el-form-item label="描述"><el-input v-model="form.description" maxlength="500" show-word-limit /></el-form-item>
        </div>
        <el-form-item label="额外标签">
          <div class="label-editor">
            <el-button class="add-label-button" :icon="Plus" plain @click="addPair('external_labels')">添加标签</el-button>
            <div v-for="(item, index) in form.external_labels" :key="`label-${index}`" class="label-row">
            <el-input v-model="item.key" aria-label="标签名" maxlength="64" />
            <el-input v-model="item.value" aria-label="标签值" maxlength="200" />
              <el-button class="label-icon-button" :icon="Minus" circle plain aria-label="删除标签" @click="form.external_labels.splice(index, 1)" />
            </div>
          </div>
        </el-form-item>

        <div class="section-heading"><div><strong>规则配置</strong></div></div>
        <el-form-item class="datasource-field" prop="datasource_ids">
          <template #label><span class="detail-label">实例分组<el-tooltip content="选择实例分组后，规则自动覆盖全部子分组中的指标实例；后续新增实例自动生效。旧规则的直接实例绑定可保留或移除。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="数据源选择说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-select v-model="datasourceSelection" multiple filterable collapse-tags :max-collapse-tags="5" placeholder="" @change="formRef?.validateField('datasource_ids')">
            <el-option-group v-if="visibleDatasourceGroups.length" label="实例分组（包含子分组）">
              <el-option v-for="item in visibleDatasourceGroups" :key="`group-${item.id}`" :label="`${item.path || item.name}（${item.scope_member_count ?? item.member_count} 个实例）`" :value="`group:${item.id}`" />
            </el-option-group>
            <el-option-group v-if="legacyDatasources.length" label="原有直接绑定（可移除）">
              <el-option v-for="item in legacyDatasources" :key="`source-${item.id}`" :label="item.name" :value="`source:${item.id}`" />
            </el-option-group>
          </el-select>
        </el-form-item>

        <div class="promql-action-row">
          <el-form-item class="promql-form-item" prop="expression">
            <template #label><span class="detail-label">PromQL<el-tooltip content="这里只填写指标查询，平台会使用下方条件判断每条瞬时数据。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="PromQL 填写说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
            <el-input v-model="form.expression" type="textarea" :autosize="{ minRows: 1 }" class="promql-input" placeholder="例如：(1 - avg by(instance) (rate(node_cpu_seconds_total{mode=&quot;idle&quot;}[5m]))) * 100" />
          </el-form-item>
          <el-button class="preview-button" type="primary" :loading="testing" :disabled="!form.expression.trim() || !effectiveDatasourceIds.length" @click="testQuery">执行预览</el-button>
        </div>

        <el-form-item class="condition-config" label="告警条件" prop="conditions">
          <div class="condition-content">
            <div class="condition-list">
            <div v-for="(condition, index) in form.conditions" :key="`condition-${index}`" class="rule-condition-row">
              <el-select v-model="condition.severity" aria-label="告警等级" @change="formRef?.validateField('conditions')">
                <el-option label="P0" value="P0" :disabled="isSeverityUsed('P0', index)" />
                <el-option label="P1" value="P1" :disabled="isSeverityUsed('P1', index)" />
                <el-option label="P2" value="P2" :disabled="isSeverityUsed('P2', index)" />
              </el-select>
              <el-select v-model="condition.operator" placeholder="运算符" aria-label="运算符" @change="formRef?.validateField('conditions')">
                <el-option v-for="operator in conditionOperators" :key="operator" :label="operator" :value="operator" />
              </el-select>
              <el-input-number v-model="condition.threshold" :controls="false" aria-label="阈值" placeholder="阈值" @change="formRef?.validateField('conditions')" />
              <el-button class="condition-remove" :icon="Minus" circle plain aria-label="删除规则条件" :disabled="form.conditions.length === 1" @click="removeCondition(index)" />
            </div>
            </div>
            <el-button class="add-condition-button" :icon="Plus" plain :disabled="form.conditions.length >= 3" @click="addCondition">添加规则条件</el-button>
          </div>
        </el-form-item>

        <el-form-item class="alert-detail-field">
          <template #label>
            <span class="detail-label">告警详情<el-tooltip content="支持变量：Prometheus 标签使用 ${labels.instance}，数据源标签使用 ${datasource_labels.env}，额外标签使用 ${external_labels.business}。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="告警详情变量说明"><QuestionFilled /></el-icon></el-tooltip></span>
          </template>
          <el-input v-model="descriptionAnnotation.value" type="textarea" :rows="4" maxlength="2000" show-word-limit placeholder="例如：${datasource_labels.env} 环境的 ${external_labels.business} 业务实例 ${labels.instance} 触发告警" />
        </el-form-item>

        <div class="policy-grid">
          <div class="policy-block lifecycle-block">
            <div class="policy-block-heading"><strong>调度与告警</strong></div>
            <el-form-item class="compact-timing-field lifecycle-policy-field" prop="for_seconds">
              <template #label><span class="detail-label">持续时间<el-tooltip content="平台在每次执行评估时确认条件状态。非零持续时间不能小于执行频率；0 表示首次命中立即触发。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="持续时间说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
              <div class="timing-control">
                <div class="interval-control"><el-input-number v-model="form.for_seconds" :min="0" :max="604800" controls-position="right" aria-label="持续时间" @change="validateTimingFields" /><span>秒</span></div>
                <small class="timing-summary" :class="{ 'is-invalid': triggerTiming.invalid }">{{ triggerTiming.text }}</small>
              </div>
            </el-form-item>
            <el-form-item class="compact-timing-field lifecycle-policy-field" prop="evaluation_interval_seconds">
              <template #label><span class="detail-label">执行频率<el-tooltip content="平台按此周期执行一次规则评估；持续时间和恢复时间会按完整评估周期向上取整。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="执行频率说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
              <div class="interval-control"><el-input-number v-model="form.evaluation_interval_seconds" :min="30" :max="86400" controls-position="right" aria-label="执行频率" @change="validateTimingFields" /><span>秒</span></div>
            </el-form-item>
            <el-form-item class="compact-timing-field lifecycle-policy-field" prop="recovery_wait_seconds">
              <template #label><span class="detail-label">恢复时间<el-tooltip content="平台在每次执行评估时确认恢复状态。非零恢复时间不能小于执行频率；0 表示首次不命中立即恢复。" placement="top"><el-icon class="detail-help-icon" tabindex="0" aria-label="恢复时间说明"><QuestionFilled /></el-icon></el-tooltip></span></template>
              <div class="timing-control">
                <div class="interval-control"><el-input-number v-model="form.recovery_wait_seconds" :min="0" :max="86400" controls-position="right" aria-label="恢复时间" @change="validateTimingFields" /><span>秒</span></div>
                <small class="timing-summary" :class="{ 'is-invalid': recoveryTiming.invalid }">{{ recoveryTiming.text }}</small>
              </div>
            </el-form-item>
            <el-form-item class="fault-center-field lifecycle-policy-field" label="故障中心" prop="fault_center_id"><el-select v-model="form.fault_center_id" filterable placeholder="选择故障中心"><el-option v-for="item in activeCenters" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item>
          </div>
        </div>

        <div class="status-line"><strong>启用规则</strong><el-switch v-model="form.status" :active-value="1" :inactive-value="0" /></div>
      </div>

      <editor-action-bar>
        <template #leading><div class="editor-action-copy"><span>规则保存到</span><strong>{{ group.name || '当前规则组' }}</strong></div></template>
        <el-button @click="cancel">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveRule">{{ isEdit ? '保存修改' : '创建规则' }}</el-button>
      </editor-action-bar>
    </el-form>

    <el-dialog v-model="previewVisible" title="数据预览" width="min(920px, 92vw)" :close-on-click-modal="false" destroy-on-close>
      <div class="preview-dialog-meta"><span>瞬时查询</span><code>{{ form.expression.trim() }}</code><time v-if="previewQueriedAt">{{ previewQueriedAt }}</time></div>
      <el-alert v-for="item in previewErrors" :key="item.datasource" class="preview-error" type="error" :closable="false" :title="`${item.datasource}：${item.message}`" show-icon />
      <el-table v-loading="testing" :data="previewRows" max-height="55vh">
        <el-table-column prop="datasource" label="数据源" min-width="150" />
        <el-table-column label="指标标签" min-width="430" show-overflow-tooltip><template #default="{ row }"><code class="metric-value">{{ row.metric }}</code></template></el-table-column>
        <el-table-column label="当前瞬时值" width="160" align="right"><template #default="{ row }"><strong class="instant-value">{{ row.value }}</strong></template></el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
      <template #footer><el-button type="primary" @click="previewVisible = false">关闭</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Minus, Plus, QuestionFilled } from '@element-plus/icons-vue'
import { getFaultCenters } from '../../api/alert'
import { createAlertRule, getAlertRule, getAlertRuleGroup, updateAlertRule } from '../../api/alertRule'
import { getDatasourceGroups } from '../../api/datasourceGroup'
import { getPromDatasources, promInstantQuery } from '../../api/prometheus'
import EditorActionBar from '../../components/EditorActionBar.vue'

const route = useRoute(), router = useRouter()
const groupId = Number(route.params.groupId), ruleId = Number(route.params.ruleId || 0), isEdit = computed(() => ruleId > 0)
const datasources = ref([]), datasourceGroups = ref([]), centers = ref([]), loading = ref(true), saving = ref(false), testing = ref(false), formRef = ref()
const previewVisible = ref(false), previewRows = ref([]), previewErrors = ref([]), previewQueriedAt = ref('')
const group = reactive({ name: '', description: '' })
const conditionOperators = ['>', '<', '>=', '<=', '!=', '==']
const severityOrder = ['P2', 'P1', 'P0']
const form = reactive({ name: '', description: '', expression: '', datasource_group_ids: [], datasource_ids: [], fault_center_id: null, evaluation_interval_seconds: 60, for_seconds: 60, recovery_wait_seconds: 0, status: 1, external_labels: [], conditions: [{ severity: 'P2', operator: '', threshold: null }] })
const descriptionAnnotation = reactive({ key: 'description', value: '' })
const formRules = {
  name: [{ required: true, message: '请输入规则名称', trigger: 'blur' }],
  expression: [{ required: true, message: '请输入 PromQL 表达式', trigger: 'blur' }],
  datasource_ids: [{ validator: (_rule, value, callback) => { if (!form.datasource_group_ids.length && !value?.length) callback(new Error('请至少选择一个实例分组')); else callback() }, trigger: 'change' }],
  conditions: [{ required: true, validator: (_rule, _value, callback) => { try { normalizeConditions(); callback() } catch (error) { callback(error) } }, trigger: 'change' }],
  evaluation_interval_seconds: [{ validator: validateEvaluationInterval, trigger: 'change' }],
  for_seconds: [{ validator: validateTimingWindow('持续时间', 604800), trigger: 'change' }],
  recovery_wait_seconds: [{ validator: validateTimingWindow('恢复时间', 86400), trigger: 'change' }],
  fault_center_id: [{ required: true, message: '请选择故障中心', trigger: 'change' }],
}
const legacyDatasources = computed(() => datasources.value.filter(item => form.datasource_ids.includes(item.id)))
const visibleDatasourceGroups = computed(() => datasourceGroups.value)
const activeCenters = computed(() => centers.value.filter(item => item.status === 1))
const datasourceSelection = computed({
  get: () => [...form.datasource_group_ids.map(id => `group:${id}`), ...form.datasource_ids.map(id => `source:${id}`)],
  set: (values) => {
    form.datasource_group_ids = values.filter(value => value.startsWith('group:')).map(value => Number(value.slice(6))).filter(Boolean)
    form.datasource_ids = values.filter(value => value.startsWith('source:')).map(value => Number(value.slice(7))).filter(Boolean)
  },
})
const effectiveDatasourceIds = computed(() => {
  const groupDatasourceIDs = datasourceGroups.value.filter(item => form.datasource_group_ids.includes(item.id)).flatMap(item => item.effective_datasource_ids || item.datasource_ids || [])
  const available = new Set(datasources.value.map(item => item.id))
  return [...new Set([...form.datasource_ids, ...groupDatasourceIDs])].filter(id => available.has(id))
})
const triggerTiming = computed(() => timingSummary(form.for_seconds, '命中', '触发'))
const recoveryTiming = computed(() => timingSummary(form.recovery_wait_seconds, '不命中', '恢复'))

function validateEvaluationInterval(_rule, value, callback) {
  const seconds = Number(value)
  if (!Number.isInteger(seconds) || seconds < 30 || seconds > 86400) return callback(new Error('执行频率必须是 30 到 86400 之间的整数秒'))
  callback()
}

function validateTimingWindow(label, maximum) {
  return (_rule, value, callback) => {
    const seconds = Number(value)
    if (!Number.isInteger(seconds) || seconds < 0 || seconds > maximum) return callback(new Error(`${label}必须是 0 到 ${maximum} 之间的整数秒`))
    const interval = Number(form.evaluation_interval_seconds)
    if (seconds > 0 && Number.isFinite(interval) && seconds < interval) return callback(new Error(`${label}不能小于执行频率；如需立即生效，请设置为 0`))
    callback()
  }
}

function validateTimingFields() {
  formRef.value?.validateField(['evaluation_interval_seconds', 'for_seconds', 'recovery_wait_seconds']).catch(() => {})
}

function timingSummary(value, stateText, actionText) {
  const seconds = Number(value)
  const interval = Number(form.evaluation_interval_seconds)
  if (seconds === 0) return { invalid: false, text: `首次${stateText}立即${actionText}` }
  if (!Number.isFinite(seconds) || !Number.isFinite(interval) || interval <= 0) return { invalid: false, text: '填写时间后显示评估次数' }
  if (seconds < interval) return { invalid: true, text: `非零时间不能小于执行频率 ${formatTimingDuration(interval)}` }
  const waitingCycles = Math.ceil(seconds / interval)
  const effectiveSeconds = waitingCycles * interval
  const rounded = effectiveSeconds !== seconds ? '（按评估周期取整）' : ''
  return { invalid: false, text: `连续 ${waitingCycles + 1} 次${stateText}后${actionText}，最早 ${formatTimingDuration(effectiveSeconds)}${rounded}` }
}

function formatTimingDuration(seconds) {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  const remainder = seconds % 60
  return [hours ? `${hours} 小时` : '', minutes ? `${minutes} 分钟` : '', remainder ? `${remainder} 秒` : ''].filter(Boolean).join(' ') || '0 秒'
}

onMounted(async () => {
  try {
    const requests = [getAlertRuleGroup(groupId), getPromDatasources(), getDatasourceGroups(), getFaultCenters()]
    if (isEdit.value) requests.push(getAlertRule(ruleId))
    const [groupResult, datasourceResult, datasourceGroupResult, centerResult, ruleResult] = await Promise.all(requests)
    Object.assign(group, groupResult.data || {})
    datasources.value = datasourceResult.data || []
    datasourceGroups.value = datasourceGroupResult.data || []
    centers.value = centerResult.data || []
    if (ruleResult) hydrateRule(ruleResult.data)
    else form.fault_center_id = (activeCenters.value.find(center => center.is_default) || activeCenters.value[0])?.id ?? null
  } catch {
    ElMessage.error('告警规则编辑页加载失败')
    cancel()
  } finally { loading.value = false }
})

function hydrateRule(rule) {
  if (!rule || Number(rule.group_id) !== groupId) { ElMessage.error('告警规则不属于当前规则组'); cancel(); return }
  const sourceConditions = Array.isArray(rule.conditions) ? rule.conditions : []
  if (!sourceConditions.length) { ElMessage.error('告警规则缺少条件配置'); cancel(); return }
  const usedSeverities = new Set()
  const conditions = []
  for (const item of sourceConditions) {
    if (conditions.length >= 3 || !severityOrder.includes(item.severity) || usedSeverities.has(item.severity)) continue
    usedSeverities.add(item.severity)
    conditions.push({ severity: item.severity, operator: conditionOperators.includes(item.operator) ? item.operator : '>', threshold: Number(item.threshold) || 0 })
  }
  if (!conditions.length) { ElMessage.error('告警规则条件配置无效'); cancel(); return }
  Object.assign(form, { name: rule.name || '', description: rule.description || '', expression: rule.expression || '', datasource_group_ids: [...(rule.datasource_group_ids || [])], datasource_ids: [...(rule.direct_datasource_ids || [])], fault_center_id: rule.fault_center_id, evaluation_interval_seconds: rule.evaluation_interval_seconds || 60, for_seconds: rule.for_seconds ?? sourceConditions[0]?.for_seconds ?? 0, recovery_wait_seconds: rule.recovery_wait_seconds ?? 0, status: rule.status, external_labels: mapToPairs(rule.external_labels), conditions })
  descriptionAnnotation.value = rule.annotations?.description || ''
}

function mapToPairs(value, excluded) { return Object.entries(value || {}).filter(([key]) => key !== excluded).map(([key, item]) => ({ key, value: item })) }
function pairsToMap(items) { return Object.fromEntries(items.map(item => [item.key.trim(), item.value.trim()]).filter(([key, value]) => key && value)) }
function addPair(type) { form[type].push({ key: '', value: '' }) }
function isSeverityUsed(severity, currentIndex) { return form.conditions.some((item, index) => index !== currentIndex && item.severity === severity) }
function addCondition() {
  if (form.conditions.length >= 3) { ElMessage.warning('P0、P1、P2 每个等级只能配置一条'); return }
  const severity = severityOrder.find(item => !form.conditions.some(condition => condition.severity === item))
  form.conditions.push({ severity, operator: '', threshold: null })
}
function removeCondition(index) {
  if (form.conditions.length > 1) {
    form.conditions.splice(index, 1)
    formRef.value?.validateField('conditions').catch(() => {})
  }
}
function normalizeConditions() {
  if (!form.conditions.length || form.conditions.length > 3) throw new Error('告警条件只能配置 1 至 3 条')
  if (new Set(form.conditions.map(item => item.severity)).size !== form.conditions.length) throw new Error('P0、P1、P2 每个等级只能配置一条')
  return form.conditions.map((item, index) => {
    if (!conditionOperators.includes(item.operator)) throw new Error(`第 ${index + 1} 个告警条件的运算符不正确`)
    if (item.threshold === null || item.threshold === undefined || !Number.isFinite(Number(item.threshold))) throw new Error(`请输入第 ${index + 1} 个告警条件的数字阈值`)
    return { severity: item.severity, operator: item.operator, threshold: Number(item.threshold) }
  })
}
function cancel() { router.push(`/alert-rules/${groupId}`) }

async function testQuery() {
  previewVisible.value = true
  previewRows.value = []
  previewErrors.value = []
  previewQueriedAt.value = new Date().toLocaleString('zh-CN', { hour12: false })
  testing.value = true
  const query = form.expression.trim()
  const results = await Promise.allSettled(effectiveDatasourceIds.value.map(async datasourceId => {
    const datasource = datasources.value.find(item => item.id === datasourceId)
    const response = await promInstantQuery({ datasource_id: datasourceId, query })
    return normalizePreviewRows(response, datasource?.name || `数据源 ${datasourceId}`)
  }))
  results.forEach((result, index) => {
    const datasourceId = effectiveDatasourceIds.value[index]
    const datasource = datasources.value.find(item => item.id === datasourceId)?.name || `数据源 ${datasourceId}`
    if (result.status === 'fulfilled') previewRows.value.push(...result.value)
    else previewErrors.value.push({ datasource, message: result.reason?.message || '查询失败' })
  })
  testing.value = false
}

function normalizePreviewRows(response, datasource) {
  const resultType = response.data?.resultType || 'vector'
  const result = response.data?.result
  if ((resultType === 'scalar' || resultType === 'string') && Array.isArray(result)) {
    return [{ datasource, metric: resultType, value: formatPreviewValue(result[1]) }]
  }
  if (!Array.isArray(result)) return []
  return result.map(item => ({ datasource, metric: formatPreviewMetric(item.metric), value: formatPreviewValue(item.value?.[1]) }))
}

function formatPreviewMetric(metric = {}) {
  const name = metric.__name__ || ''
  const labels = Object.entries(metric).filter(([key]) => key !== '__name__').map(([key, value]) => `${key}="${value}"`)
  if (!labels.length) return name || '{}'
  return `${name}{${labels.join(', ')}}`
}

function formatPreviewValue(value) {
  if (value === undefined || value === null || value === '') return '-'
  const number = Number(value)
  return Number.isFinite(number) ? number.toLocaleString('zh-CN', { maximumFractionDigits: 8 }) : String(value)
}

async function saveRule() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  let conditions
  try { conditions = normalizeConditions() } catch (error) { ElMessage.error(error.message); return }
  saving.value = true
  try {
    const annotations = { description: descriptionAnnotation.value.trim() }
    const payload = { group_id: groupId, name: form.name.trim(), description: form.description.trim(), expression: form.expression.trim(), datasource_group_ids: form.datasource_group_ids, datasource_ids: form.datasource_ids, fault_center_id: form.fault_center_id, severity: conditions[0].severity, evaluation_interval_seconds: form.evaluation_interval_seconds, for_seconds: form.for_seconds, recovery_wait_seconds: form.recovery_wait_seconds, status: form.status, conditions, external_labels: pairsToMap(form.external_labels), annotations: Object.fromEntries(Object.entries(annotations).filter(([, value]) => value)) }
    if (isEdit.value) await updateAlertRule(ruleId, payload)
    else await createAlertRule(payload)
    ElMessage.success(isEdit.value ? '告警规则已更新' : '告警规则已创建')
    cancel()
  } finally { saving.value = false }
}
</script>

<style scoped>
.source-card.selected { border-color: var(--ops-primary); background: var(--ops-primary-soft); box-shadow: 0 0 0 1px var(--ops-primary); }
.source-card:hover { border-color: var(--ops-primary); }
.source-card:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 3px; }

.vm-logo { filter: invert(1); }
:global(html.dark) .vm-logo { filter: none; }
.rule-editor-page { --line: #e5e9f0; --muted:var(--ops-muted); display: flex; width: 100%; min-width: 0; flex-direction: column; gap: 16px; padding-bottom: 72px; }
.preview-header { display: flex; align-items: center; justify-content: space-between; }
.editor-form { display: flex; flex-direction: column; gap: 16px; }.rule-config-card { padding: 22px 24px; border: 1px solid var(--ops-border); border-radius: 8px; background: var(--ops-surface); }.section-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 20px; padding-bottom: 14px; border-bottom: 1px solid var(--ops-border-soft); }.section-heading > div { display: flex; align-items: baseline; gap: 12px; }.section-heading strong { color: #344054; font-size: 14px; font-weight: 600; }.section-heading span, .section-heading small { color: var(--muted); font-size: 12px; }.two-column { display: grid; grid-template-columns: 1fr 1fr; gap: 0 18px; }.full-row { grid-column: 1 / -1; }
.label-editor { display: flex; width: 100%; flex-direction: column; gap: 8px; }.add-label-button { width: 100%; border-style: dashed; }.label-row { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) 28px; align-items: center; gap: 8px; }.label-icon-button { width: 26px; height: 26px; color: #8a94a3; background: var(--ops-surface); border-color: var(--ops-border); }.label-icon-button :deep(.el-icon) { font-size: 14px; }.label-icon-button:hover, .label-icon-button:focus-visible { color: #606b7b; background: var(--ops-surface-soft); border-color: #c8cdd5; }.field-label { margin-bottom: 10px; color: #606b7b; font-size: 13px; }.source-cards { display: flex; flex-wrap: wrap; gap: 12px; }.source-card { position: relative; display: grid; width: 132px; height: 108px; place-items: center; padding: 10px; border: 1px solid var(--ops-border); border-radius: 8px; background: var(--ops-surface); cursor: pointer; font: inherit; }.source-card img { width: 64px; height: 42px; object-fit: contain; }.source-card span { color: var(--ops-text); font-size: 12px; font-weight: 600; }.source-card small { color: var(--muted); font-size: 10px; }.source-card > .el-icon { position: absolute; top: 7px; right: 7px; color: var(--ops-primary); }.inherited-datasources { display: flex; width: 100%; min-height: 76px; flex-direction: column; gap: 10px; margin-top: 10px; padding: 14px 16px; border: 1px solid #dbe7f5; border-radius: 8px; background: var(--ops-primary-soft); }.inherited-datasources > div:first-child { display: flex; align-items: baseline; gap: 10px; }.inherited-datasources strong { color: #344054; font-size: 13px; }.inherited-datasources span { color: #8a96a8; font-size: 11px; }.inherited-members { display: flex; flex-wrap: wrap; gap: 6px; }.datasource-field { margin-top: 20px; }.datasource-field :deep(.el-select), .annotation-grid :deep(.el-select) { width: 100%; }.field-help { margin-top: 7px; color: var(--muted); font-size: 11px; line-height: 1.6; }
.promql-action-row { display: flex; align-items: flex-start; gap: 10px; }.promql-form-item { min-width: 0; flex: 1; }.promql-input :deep(.el-textarea__inner) { min-height: 32px !important; overflow-y: hidden !important; resize: none !important; color: #344054; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; line-height: 1.5; }.preview-button { flex: none; margin-top: 30px; }
.condition-config { margin-top: 20px; }.condition-list { display: flex; flex-direction: column; gap: 10px; }.rule-condition-row { display: grid; grid-template-columns: 140px 90px minmax(140px, 1fr) 30px; align-items: center; gap: 10px; }.rule-condition-row :deep(.el-select), .rule-condition-row :deep(.el-input-number) { width: 100%; }.condition-remove { width: 26px; height: 26px; color: #8a94a3; }.condition-remove :deep(.el-icon) { font-size: 14px; }.add-condition-button { width: 100%; margin-top: 10px; border-style: dashed; }.evaluation-plan { display: flex; flex-direction: column; gap: 18px; margin-top: 22px; padding-top: 20px; border-top: 1px solid var(--ops-border-soft); }.plan-row { display: grid; grid-template-columns: 88px minmax(0, 1fr); align-items: center; gap: 16px; }.plan-row-schedule { align-items: start; }.plan-label { padding-top: 1px; color: #475467; font-size: 13px; font-weight: 500; }.interval-control { display: grid; width: 100%; grid-template-columns: minmax(0, 1fr) 38px; align-items: center; overflow: hidden; border: 1px solid var(--ops-border); border-radius: 6px; background: var(--ops-surface); transition: border-color .2s, box-shadow .2s; }.interval-control:focus-within { border-color: var(--ops-primary); box-shadow: 0 0 0 2px rgba(64,158,255,.1); }.interval-control :deep(.el-input-number) { width: 100%; }.interval-control :deep(.el-input__wrapper) { box-shadow: none; }.interval-control > span { color: var(--ops-subtle); font-size: 12px; text-align: center; }.schedule-content { min-width: 0; }.schedule-controls { display: grid; grid-template-columns: minmax(420px, 1fr) 280px; gap: 12px; }.schedule-controls :deep(.el-checkbox-group) { display: grid; grid-template-columns: repeat(7, minmax(48px, 1fr)); gap: 6px; }.schedule-controls :deep(.el-checkbox-button) { min-width: 0; }.schedule-controls :deep(.el-checkbox-button__inner) { width: 100%; padding: 8px 5px; border: 1px solid #dfe3e8 !important; border-radius: 6px !important; box-shadow: none !important; color: var(--ops-muted); font-size: 12px; }.schedule-controls :deep(.el-checkbox-button.is-checked .el-checkbox-button__inner) { border-color: var(--ops-primary) !important; background: var(--ops-primary-soft); color: #1677d2; font-size: 12px; }.schedule-controls :deep(.el-date-editor) { width: 100%; }.plan-hint { margin-top: 7px; color: var(--ops-subtle); font-size: 11px; }.status-line { margin-top: 20px; display: flex; align-items: center; justify-content: space-between; padding: 15px 16px; border: 1px solid #e6eaf0; border-radius: 7px; background: var(--ops-surface-soft); }.status-line strong { color:#344054; font-size:13px; }
.timing-control { width: 100%; }
.timing-summary { display: block; min-height: 17px; margin-top: 5px; color: var(--ops-muted); font-size: 11px; line-height: 17px; }
.timing-summary.is-invalid { color: var(--el-color-danger); }
.policy-block { display: flex; flex-direction: column; gap: 16px; margin-top: 20px; padding: 4px 0 0; }
.policy-block-heading { display: flex; align-items: baseline; gap: 10px; padding-bottom: 12px; border-bottom: 1px solid #e8ecf1; }
.policy-block-heading strong { color: #344054; font-size: 14px; font-weight: 600; }
.policy-block-heading span { color: var(--ops-subtle); font-size: 11px; }
.compact-timing-field { display: grid; grid-template-columns: 120px minmax(220px, 320px); align-items: center; gap: 16px; margin-bottom: 0; }
.compact-timing-field :deep(.el-form-item__label) { height: auto; margin: 0; padding: 0; line-height: 1.4; }
.compact-timing-field :deep(.el-form-item__content) { min-width: 0; }
.evaluation-plan.policy-block { gap: 16px; margin-top: 20px; padding: 4px 0 0; border: 0; }
.evaluation-plan .plan-row { display: grid; grid-template-columns: 120px minmax(0, 1fr); align-items: start; gap: 16px; }
.policy-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 40px; margin-top: 0; }
.policy-grid .policy-block { min-width: 0; margin-top: 0; padding: 0; }
.policy-grid .lifecycle-block { display: grid; grid-column: 1 / -1; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: start; gap: 16px 40px; }
.policy-grid .lifecycle-block .policy-block-heading { grid-column: 1 / -1; }
.policy-grid .compact-timing-field { grid-template-columns: minmax(0, 1fr); gap: 8px; }
.policy-grid .compact-timing-field :deep(.el-form-item__label) { height: 20px; justify-content: flex-start; }
.policy-grid .compact-timing-field :deep(.el-form-item__label::before) { display: none !important; }
.policy-grid .fault-center-field :deep(.el-form-item__label::before) { display: none !important; }
.policy-grid .compact-timing-field :deep(.el-form-item__content), .policy-grid .timing-control, .policy-grid .interval-control { width: 100%; }
.policy-grid .detail-label { display: flex; align-items: center; }
.policy-grid .plan-label { display: flex; min-height: 20px; align-items: center; gap: 5px; padding-top: 0; }
.policy-grid .plan-label::before { width: 8px; flex: none; margin-right: 4px; content: '*'; visibility: hidden; }
.policy-grid .schedule-controls { width: 100%; grid-template-columns: minmax(0, 500px) minmax(250px, 1fr); gap: 10px; }
.policy-grid .schedule-controls :deep(.el-date-editor) { max-width: none; }
.condition-content { width: 100%; }
.condition-config :deep(.el-form-item__content) { display: block; }
.alert-detail-field { margin-top: 20px; }
.detail-label { display: inline-flex; align-items: center; gap: 5px; }
.detail-help-icon { color: var(--ops-subtle); cursor: help; font-size: 14px; }
.detail-help-icon:hover, .detail-help-icon:focus-visible { color: var(--ops-primary); outline: none; }
.alert-detail-field :deep(.el-textarea__inner::placeholder) { color: #a8abb2; }
.evaluation-interval-field { margin-bottom: 0; padding-bottom: 22px; }
.lifecycle-policy-field { margin-bottom: 0; }
.fault-center-field { margin-top: 22px; }
.fault-center-field :deep(.el-select) { width: 100%; }
.policy-grid .fault-center-field { margin: 0; }
.schedule-controls :deep(.el-select) { width: 100%; }
.preview-dialog-meta { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 10px; margin-bottom: 14px; color: #8a96a8; font-size: 11px; }.preview-dialog-meta code { overflow: hidden; padding: 7px 9px; border-radius: 5px; background: var(--ops-surface-soft); color: #475467; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }.preview-error { margin-bottom: 10px; }.metric-value { color: #475467; font-size: 12px; }.instant-value { color: var(--ops-primary); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
@media (max-width: 1100px) { .rule-condition-row { grid-template-columns: 120px 80px minmax(120px, 1fr) 30px; } }
@media (max-width: 1000px) { .policy-grid, .policy-grid .lifecycle-block { grid-template-columns: 1fr; }.schedule-controls { grid-template-columns: 1fr; }.schedule-controls :deep(.el-date-editor) { max-width: 320px; }.policy-grid .schedule-controls :deep(.el-date-editor) { max-width: none; } }
@media (max-width: 760px) { .rule-condition-row { grid-template-columns: 1fr 1fr 30px; }.rule-condition-row > :nth-child(3) { grid-column: 1 / 3; }.condition-remove { grid-column: 3; grid-row: 1 / 3; } }
@media (max-width: 620px) { .rule-config-card { padding: 18px 16px; }.two-column { grid-template-columns: 1fr; }.promql-action-row { flex-direction: column; }.preview-button { width: 100%; margin-top: -8px; }.plan-row { grid-template-columns: 1fr; gap: 8px; }.interval-control { width: 100%; }.schedule-controls :deep(.el-checkbox-group) { grid-template-columns: repeat(4, 1fr); }.schedule-controls :deep(.el-date-editor) { max-width: none; }.section-heading, .section-heading > div { align-items: flex-start; flex-direction: column; }.preview-dialog-meta { grid-template-columns: 1fr; }.preview-dialog-meta time { display: none; } }
@media (max-width: 620px) { .policy-block { padding: 0; }.policy-block-heading { align-items: flex-start; flex-direction: column; gap: 4px; }.policy-grid .compact-timing-field, .policy-grid .evaluation-plan .plan-row, .policy-grid .schedule-controls { grid-template-columns: 1fr; gap: 8px; } }
</style>

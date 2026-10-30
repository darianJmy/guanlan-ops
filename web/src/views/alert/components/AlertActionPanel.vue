<template>
  <section class="action-panel">
    <div class="strategy-toolbar">
      <div><el-input v-model="ruleKeyword" clearable aria-label="搜索自动化处置规则" placeholder="搜索策略名称" @keyup.enter="searchRules" @clear="searchRules"><template #append><el-button aria-label="搜索" @click="searchRules"><el-icon><Search /></el-icon></el-button></template></el-input></div>
      <div><el-button :icon="Refresh" @click="loadRules">刷新</el-button><el-button type="primary" :icon="Plus" @click="openRule()">创建</el-button></div>
    </div>
    <el-table v-loading="rulesLoading" :data="pagedRules">
      <el-table-column prop="name" label="规则名称" min-width="170" show-overflow-tooltip />
      <el-table-column label="匹配告警" min-width="230" show-overflow-tooltip>
        <template #default="{ row }"><code>{{ matcherSummary(row.matchers) }}</code></template>
      </el-table-column>
      <el-table-column label="触发时机" width="110" align="center"><template #default="{ row }"><el-tag :type="row.trigger_status === 'firing' ? 'danger' : 'success'" effect="dark">{{ triggerText(row.trigger_status) }}</el-tag></template></el-table-column>
      <el-table-column label="自动化任务" min-width="180" show-overflow-tooltip><template #default="{ row }">{{ actionText(row) }}</template></el-table-column>
      <el-table-column label="执行保护" min-width="210" show-overflow-tooltip><template #default="{ row }">{{ safetyText(row) }}</template></el-table-column>
      <el-table-column label="状态" width="90" align="center"><template #default="{ row }"><el-tag :type="row.status === 1 ? 'success' : 'info'" effect="plain">{{ row.status === 1 ? '已启用' : '已停用' }}</el-tag></template></el-table-column>
      <el-table-column label="操作" width="130" fixed="right"><template #default="{ row }"><el-button link type="primary" @click="openRule(row)">编辑</el-button><el-button link type="danger" @click="removeRule(row)">删除</el-button></template></el-table-column>
      <template #empty><TableEmpty /></template>
    </el-table>
    <div class="pagination"><el-pagination v-model:current-page="rulePage" v-model:page-size="rulePageSize" :page-sizes="[10, 20, 50]" :total="filteredRules.length" layout="total, sizes, prev, pager, next" @size-change="rulePage = 1" /></div>

    <el-drawer v-model="drawerVisible" class="center-drawer config-drawer action-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <template #header><div class="drawer-header"><DrawerBackButton @click="drawerVisible = false" /><span class="drawer-title">{{ form.id ? '编辑处置规则' : '创建处置规则' }}</span></div></template>
      <el-form label-position="top" class="strategy-form action-form">
        <el-form-item label="规则名称" required>
          <el-input v-model="form.name" maxlength="128" show-word-limit aria-label="处置规则名称" />
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">匹配标签<el-tooltip content="标签来自系统标签、PromQL 标签、数据源标签和额外标签；多项之间是且关系" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <div class="label-match-list">
            <div v-for="(matcher, index) in form.matchers" :key="index" class="label-match-item">
              <el-button v-if="form.matchers.length > 1" class="label-match-remove" text circle type="danger" :icon="Delete" :aria-label="`删除匹配条件 ${index + 1}`" @click="removeMatcher(index)" />
              <div class="label-source-control"><span>标签来源</span><el-radio-group v-model="matcher.namespace" class="label-source-options" aria-label="标签来源" @change="matcher.key = ''"><el-radio-button v-for="option in namespaceOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group></div>
              <div class="label-match-fields"><div class="label-field-control"><span>标签 Key</span><el-select v-if="matcher.namespace === 'system_labels'" v-model="matcher.key" aria-label="系统标签"><el-option v-for="option in systemLabelOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select><el-input v-else v-model="matcher.key" maxlength="64" aria-label="标签 Key" /></div><div class="label-field-control"><span>标签 Value</span><el-input v-model="matcher.value" maxlength="255" aria-label="标签 Value" /></div></div>
            </div>
            <el-button class="add-match-condition" plain :icon="Plus" :disabled="form.matchers.length >= 10" @click="addMatcher">添加匹配条件</el-button>
          </div>
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">触发时机<el-tooltip content="仅在告警状态发生转换时触发，不随每次规则评估重复执行" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-radio-group v-model="form.trigger_status" class="full-radio-options" aria-label="触发时机"><el-radio-button value="firing">进入告警中</el-radio-button><el-radio-button value="resolved">进入已恢复</el-radio-button></el-radio-group>
        </el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">自动化任务<el-tooltip content="处置规则只负责匹配故障并触发；脚本、执行节点和超时由自动化任务管理" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <el-select v-model="form.target_id" filterable aria-label="自动化任务"><el-option v-for="item in enabledTasks" :key="item.id" :label="item.name" :value="item.id" /></el-select>
        </el-form-item>

        <div class="safety-block">
          <h4>执行保护</h4>
          <el-form-item label="静默或抑制时跳过"><el-switch v-model="form.skip_suppressed" /><div class="form-help">默认跳过被静默或抑制的告警，不启动自动化任务。</div></el-form-item>
          <el-form-item label="冷却时间"><div class="number-field"><el-input-number v-model="form.cooldown_seconds" :min="0" :max="2592000" :step="60" controls-position="right" /><span>秒</span></div></el-form-item>
        </div>
        <div class="safety-block">
          <h4>执行结果通知</h4>
          <div class="notify-options"><el-checkbox v-model="form.notify_on_failure">执行失败时通知</el-checkbox><el-checkbox v-model="form.notify_on_success">执行成功时通知</el-checkbox></div>
          <el-form-item v-if="form.notify_on_failure || form.notify_on_success" label="通知对象" required><el-select v-model="form.notification_channel_id" filterable><el-option v-for="item in enabledChannels" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item>
        </div>
        <el-form-item label="启用状态"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button @click="drawerVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveRule">保存</el-button></div></template>
    </el-drawer>
  </section>
</template>

<script setup>
import DrawerBackButton from '../../../components/DrawerBackButton.vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ArrowLeft, Delete, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { createAlertActionRule, deleteAlertActionRule, getAlertActionRules, updateAlertActionRule } from '../../../api/alert'
import { getAutomationTaskOptions } from '../../../api/automationTask'
import { getNotificationChannels } from '../../../api/notification'

const props = defineProps({ centerId: { type: Number, required: true } })
const emptyMatcher = () => ({ namespace: 'labels', key: '', value: '' })
const emptyForm = () => ({ id: null, name: '', matchers: [emptyMatcher()], trigger_status: 'firing', target_id: null, skip_suppressed: true, cooldown_seconds: 300, notify_on_failure: true, notify_on_success: false, notification_channel_id: null, enabled: true })
const namespaceOptions = [{ label: '系统标签', value: 'system_labels' }, { label: 'PromQL 标签', value: 'labels' }, { label: '数据源标签', value: 'datasource_labels' }, { label: '额外标签', value: 'external_labels' }]
const systemLabelOptions = [{ label: 'alertname（告警名称）', value: 'alertname' }, { label: 'severity（告警等级）', value: 'severity' }, { label: 'datasource（数据源）', value: 'datasource' }]
const drawerVisible = ref(false), rulesLoading = ref(false), saving = ref(false)
const rules = ref([]), tasks = ref([]), channels = ref([])
const ruleKeyword = ref('')
const rulePage = ref(1), rulePageSize = ref(10)
const form = reactive(emptyForm())
const filteredRules = computed(() => {
  const keyword = ruleKeyword.value.trim().toLowerCase()
  if (!keyword) return rules.value
  return rules.value.filter(row => `${row.name} ${matcherSummary(row.matchers)} ${triggerText(row.trigger_status)} ${actionText(row)}`.toLowerCase().includes(keyword))
})
const pagedRules = computed(() => {
  const pageCount = Math.max(1, Math.ceil(filteredRules.value.length / rulePageSize.value))
  const currentPage = Math.min(rulePage.value, pageCount)
  const start = (currentPage - 1) * rulePageSize.value
  return filteredRules.value.slice(start, start + rulePageSize.value)
})
const enabledTasks = computed(() => tasks.value.filter(item => item.status === 1))
const enabledChannels = computed(() => channels.value.filter(item => item.status === 1))

onMounted(async () => {
  await Promise.all([loadRules(), loadDependencies()])
})

async function loadRules() { rulesLoading.value = true; try { rules.value = (await getAlertActionRules(props.centerId)).data || []; rulePage.value = Math.min(rulePage.value, Math.max(1, Math.ceil(rules.value.length / rulePageSize.value))) } finally { rulesLoading.value = false } }
async function searchRules() { rulePage.value = 1; await loadRules() }
async function loadDependencies() { try { const [taskResponse, channelResponse] = await Promise.all([getAutomationTaskOptions(), getNotificationChannels()]); tasks.value = taskResponse.data || []; channels.value = channelResponse.data || [] } catch { tasks.value = []; channels.value = [] } }
function openRule(row) {
  Object.assign(form, emptyForm(), row ? { id: row.id, name: row.name, matchers: (row.matchers || []).map(item => ({ ...item })), trigger_status: row.trigger_status, target_id: row.action_type === 'automation_task' ? row.target_id : null, skip_suppressed: row.skip_suppressed, cooldown_seconds: row.cooldown_seconds, notify_on_failure: row.notify_on_failure, notify_on_success: row.notify_on_success, notification_channel_id: row.notification_channel_id || null, enabled: row.status === 1 } : {})
  if (!form.matchers.length) form.matchers = [emptyMatcher()]
  drawerVisible.value = true
}
function addMatcher() { if (form.matchers.length < 10) form.matchers.push(emptyMatcher()); else ElMessage.warning('匹配标签最多配置 10 组') }
function removeMatcher(index) { form.matchers.splice(index, 1); if (!form.matchers.length) form.matchers.push(emptyMatcher()) }
function normalizedPayload() {
  return {
    name: form.name.trim(),
    matchers: form.matchers.map(item => ({ namespace: item.namespace, key: item.key.trim(), value: item.value.trim() })),
    trigger_status: form.trigger_status, target_id: Number(form.target_id || 0),
    skip_suppressed: form.skip_suppressed,
    cooldown_seconds: Number(form.cooldown_seconds || 0), notify_on_failure: form.notify_on_failure, notify_on_success: form.notify_on_success,
    notification_channel_id: Number(form.notification_channel_id || 0), status: form.enabled ? 1 : 0,
  }
}
async function saveRule() {
  if (!form.name.trim()) return ElMessage.warning('请输入规则名称')
  if (form.matchers.some(item => !item.namespace || !item.key.trim() || !item.value.trim())) return ElMessage.warning('请完整填写匹配标签')
  if (!form.target_id) return ElMessage.warning('请选择自动化任务')
  if ((form.notify_on_failure || form.notify_on_success) && !form.notification_channel_id) return ElMessage.warning('请选择通知对象')
  saving.value = true
  try {
    const payload = normalizedPayload()
    if (form.id) await updateAlertActionRule(props.centerId, form.id, payload)
    else await createAlertActionRule(props.centerId, payload)
    await loadRules(); drawerVisible.value = false; ElMessage.success(form.id ? '处置规则已更新' : '处置规则已创建')
  } finally { saving.value = false }
}
async function removeRule(row) { await ElMessageBox.confirm(`确认删除处置规则“${row.name}”？尚未执行的任务也会取消。`, '删除规则', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' }); await deleteAlertActionRule(props.centerId, row.id); await loadRules(); ElMessage.success('处置规则已删除') }

function matcherSummary(values) { return (values || []).map(item => `${item.namespace}.${item.key}:${item.value}`).join('，') || '—' }
function triggerText(value) { return value === 'resolved' ? '进入已恢复' : '进入告警中' }
function actionText(row) { if (row.action_type !== 'automation_task') return '旧配置，请重新绑定自动化任务'; return tasks.value.find(item => item.id === row.target_id)?.name || `自动化任务 #${row.target_id}` }
function safetyText(row) { const values = []; if (row.skip_suppressed) values.push('跳过静默/抑制'); if (row.cooldown_seconds) values.push(`冷却 ${formatDuration(row.cooldown_seconds)}`); if (row.notify_on_failure) values.push('失败通知'); if (row.notify_on_success) values.push('成功通知'); return values.join(' · ') || '直接执行' }
function formatDuration(seconds) { const value = Number(seconds || 0); if (value >= 3600 && value % 3600 === 0) return `${value / 3600} 小时`; if (value >= 60 && value % 60 === 0) return `${value / 60} 分钟`; return `${value} 秒` }
</script>

<style scoped>
.action-panel{min-width:0}
.strategy-toolbar{display:flex;align-items:center;justify-content:space-between;gap:16px}
.strategy-toolbar>div{display:flex;align-items:center;gap:8px}
.strategy-toolbar .el-input{width:200px}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;color:#48566a;font-size:12px}
.pagination{display:flex;justify-content:flex-end;margin-top:18px}
.drawer-header{display:flex;width:100%;align-items:center;gap:12px}
.back-button{height:32px;padding:0 8px;color:var(--ops-primary);font-size:14px}
.drawer-title{color:var(--ops-text);font-size:16px;font-weight:600}
.action-form{width:100%}
.action-form :deep(.el-form-item){margin-bottom:24px}
.action-form :deep(.el-select){width:100%}
.form-label{display:inline-flex;align-items:center;gap:6px;color:var(--ops-text);font-weight:500}
.form-label .el-icon{color:var(--ops-muted);cursor:help}
.form-help{width:100%;margin-top:7px;color:#8a93a3;font-size:12px;line-height:1.55}
.label-match-list{display:flex;width:100%;flex-direction:column;gap:10px}
.label-match-item{position:relative;padding:12px;background:#f7f9fc;border-radius:4px}
.label-match-remove{position:absolute;z-index:1;top:4px;right:5px;width:28px;height:28px}
.label-source-control{display:flex;flex-direction:column;gap:6px}
.label-source-control>span,.label-field-control>span{color:#8a95a5;font-size:11px}
.label-source-options{display:flex;width:100%}
.label-source-options :deep(.el-radio-button){flex:1}
.label-source-options :deep(.el-radio-button__inner){width:100%;padding-inline:8px}
.label-match-fields{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:10px;margin-top:10px}
.label-field-control{display:flex;min-width:0;flex-direction:column;gap:6px}
.label-field-control .el-input{width:100%}
.add-match-condition{width:100%;border-style:dashed}
.full-radio-options{display:flex;width:100%}
.full-radio-options :deep(.el-radio-button){flex:1}
.full-radio-options :deep(.el-radio-button__inner){width:100%}
.safety-block{margin:4px 0 22px;padding:18px 18px 1px;background:#f7f9fc;border-radius:6px}
.safety-block h4{margin:0 0 18px;color:#364257;font-size:14px}
.safety-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px}
.notify-options{display:flex;gap:24px;margin-bottom:18px}
.number-field{display:flex;width:100%;align-items:center}
.number-field :deep(.el-input-number){min-width:0;flex:1;width:100%}
.number-field>span{height:32px;flex:0 0 auto;padding:0 13px;color:var(--ops-muted);background:var(--ops-surface-soft);border:1px solid var(--ops-border);border-left:0;border-radius:0 4px 4px 0;font-size:12px;line-height:30px;white-space:nowrap}
.number-field :deep(.el-input-number .el-input__wrapper){border-radius:4px 0 0 4px}
.drawer-footer{display:flex;justify-content:flex-end;gap:8px}
:global(.action-drawer.el-drawer){max-width:100vw;border-radius:0!important}
:global(.action-drawer .el-drawer__header){margin-bottom:0;padding:18px 20px;border-bottom:1px solid #e5e9f2}
:global(.action-drawer .el-drawer__body){padding:18px 20px;background:var(--ops-surface)}
:global(.action-drawer .el-drawer__footer){padding:12px 20px;border-top:1px solid #e5e9f2}
@media(max-width:900px){.strategy-toolbar{align-items:stretch;flex-direction:column}.strategy-toolbar>div,.strategy-toolbar .el-input{width:100%}.strategy-toolbar>div:last-child{justify-content:flex-end}.safety-grid{grid-template-columns:1fr}}
@media(max-width:600px){.label-match-fields{grid-template-columns:1fr}}
</style>

<template>
  <section class="action-panel">
    <div class="strategy-toolbar">
      <div><el-input v-model="keyword" clearable :prefix-icon="Search" placeholder="搜索分析规则" aria-label="搜索分析规则" @input="page = 1" /><el-select v-model="status" clearable placeholder="启用状态" aria-label="筛选启用状态" style="width:130px" @change="page = 1"><el-option label="已启用" :value="1" /><el-option label="已停用" :value="0" /></el-select></div>
      <div><el-button :icon="Refresh" @click="loadRules">刷新</el-button><el-button v-if="canEdit" type="primary" :icon="Plus" @click="openRule()">创建</el-button></div>
    </div>
    <el-table v-loading="loading" :data="pagedRules">
      <el-table-column prop="name" label="规则名称" min-width="160" show-overflow-tooltip />
      <el-table-column label="匹配标签" min-width="240" show-overflow-tooltip><template #default="{ row }"><code>{{ matcherSummary(row.matchers) }}</code></template></el-table-column>
      <el-table-column label="模型" min-width="150"><template #default="{ row }">{{ row.model_name || models.find(m => m.id === row.model_id)?.name || `模型 #${row.model_id}` }}</template></el-table-column>
      <el-table-column prop="priority" label="优先级" width="90" />
      <el-table-column label="状态" width="90"><template #default="{ row }"><el-tag :type="row.status ? 'success' : 'info'">{{ row.status ? '已启用' : '已停用' }}</el-tag></template></el-table-column>
      <el-table-column label="最近分析" min-width="170"><template #default="{ row }">{{ row.latest_analysis_at ? new Date(row.latest_analysis_at).toLocaleString('zh-CN', { hour12: false }) : '—' }}</template></el-table-column>
      <el-table-column v-if="canEdit" label="操作" width="120"><template #default="{ row }"><el-button link type="primary" @click="openRule(row)">编辑</el-button><el-button link type="danger" @click="removeRule(row)">删除</el-button></template></el-table-column>
      <template #empty><TableEmpty /></template>
    </el-table>
    <div class="pagination"><el-pagination v-model:current-page="page" :page-size="10" :total="filteredRules.length" layout="total, prev, pager, next" /></div>
    <el-drawer v-model="drawerVisible" class="center-drawer config-drawer ai-analysis-drawer" size="min(460px, 94vw)" :close-on-click-modal="false" :show-close="true" destroy-on-close>
      <template #header><span class="drawer-title">{{ form.id ? '编辑分析规则' : '创建分析规则' }}</span></template>
      <el-form label-position="top" class="action-form" :disabled="saving">
        <el-form-item label="规则名称" required><el-input v-model="form.name" maxlength="128" aria-label="分析规则名称" /></el-form-item>
        <el-form-item required>
          <template #label><span class="form-label">匹配标签<el-tooltip content="多项条件之间是且关系。同名标签优先级：系统标签 > 告警标签 > 数据源标签 > 额外标签。只使用最高优先级的值，不匹配时不会继续尝试其他来源。" placement="top"><el-icon><QuestionFilled /></el-icon></el-tooltip></span></template>
          <AlertLabelCondition v-model="form.matchers" label="匹配条件" />
        </el-form-item>
        <el-form-item label="使用模型" required><el-select v-model="form.model_id" placeholder=" " filterable><el-option v-for="m in models" :key="m.id" :label="m.name" :value="m.id" /></el-select></el-form-item>
        <el-form-item label="额外允许查询的数据源"><el-select v-model="form.allowed_datasource_ids" multiple filterable placeholder=" "><el-option v-for="d in datasources" :key="d.id" :label="d.name" :value="d.id" /></el-select></el-form-item>
        <el-form-item label="优先级"><el-input-number v-model="form.priority" class="priority-input" :min="0" :max="10000" /></el-form-item>
        <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      </el-form>
      <template #footer><div class="drawer-footer"><el-button :disabled="saving" @click="drawerVisible = false">取消</el-button><el-button type="primary" :loading="saving" @click="saveRule">保存</el-button></div></template>
    </el-drawer>
  </section>
</template>
<script setup>
import AlertLabelCondition from '../../../components/AlertLabelCondition.vue'
import { computed, reactive, ref, watch } from 'vue'
import { Delete, Plus, QuestionFilled, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import TableEmpty from '../../../components/TableEmpty.vue'
import { useUserStore } from '../../../stores/user'
import { getAlertAIRules, saveAlertAIRule, deleteAlertAIRule, getAlertAIOptions } from '../../../api/alertAI'
const props = defineProps({ centerId: { type: Number, required: true } })
const user = useUserStore()
const canEdit = computed(() => user.hasPermission('alert:ai-analysis:edit') && user.hasPermission('datasource:list'))
const emptyMatcher = () => ({ namespace: 'all_labels', key: '', value: '' })
const defaults = () => ({ id: null, name: '', matchers: [emptyMatcher()], model_id: null, allowed_datasource_ids: [], priority: 100, enabled: true })
const form = reactive(defaults())
const rules = ref([]), models = ref([]), datasources = ref([]), keyword = ref(''), status = ref(''), page = ref(1)
const loading = ref(false), saving = ref(false), drawerVisible = ref(false)
const systemLabelOptions = [{ label: 'alertname（告警名称）', value: 'alertname' }, { label: 'severity（告警等级）', value: 'severity' }, { label: 'datasource（数据源）', value: 'datasource' }]
const matcherSummary = values => (values || []).map(v => `${v.key}=${v.value}`).join('，')
const filteredRules = computed(() => rules.value.filter(r => (status.value === '' || status.value == null || r.status === status.value) && `${r.name} ${matcherSummary(r.matchers)}`.toLowerCase().includes(keyword.value.trim().toLowerCase())))
const pagedRules = computed(() => filteredRules.value.slice((page.value - 1) * 10, page.value * 10))
watch(() => props.centerId, async () => { if (props.centerId) await loadRules() }, { immediate: true })
async function loadRules() { loading.value = true; try { rules.value = (await getAlertAIRules(props.centerId)).data || []; page.value = Math.min(page.value, Math.max(1, Math.ceil(rules.value.length / 10))); if (canEdit.value) { const data = (await getAlertAIOptions(props.centerId)).data || {}; models.value = data.models || []; datasources.value = data.datasources || [] } } finally { loading.value = false } }
function openRule(row) { Object.assign(form, defaults(), row ? { ...row, enabled: row.status === 1, matchers: row.matchers.map(m => ({ ...m })), allowed_datasource_ids: [...row.allowed_datasource_ids] } : {}); drawerVisible.value = true }
async function saveRule() {
  if (!form.name.trim() || !form.model_id || form.matchers.some(m => !m.key.trim() || !m.value.trim())) return ElMessage.warning('请填写名称、匹配标签和模型')
  saving.value = true
  try { await saveAlertAIRule(props.centerId, form.id, { name: form.name.trim(), matchers: form.matchers.map(item => ({ ...item, namespace: 'all_labels' })), model_id: form.model_id, allowed_datasource_ids: form.allowed_datasource_ids, priority: form.priority, status: form.enabled ? 1 : 0 }); drawerVisible.value = false; await loadRules(); ElMessage.success('分析规则已保存') } finally { saving.value = false }
}
async function removeRule(row) { await ElMessageBox.confirm(`确认删除分析规则“${row.name}”？历史报告会保留。`, '删除规则', { type: 'warning' }); await deleteAlertAIRule(props.centerId, row.id); await loadRules() }
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
.priority-input{width:100%}
.form-label{display:inline-flex;align-items:center;gap:6px;color:var(--ops-text);font-weight:500}
.form-label .el-icon{color:var(--ops-muted);cursor:help}
.form-help{width:100%;margin-top:7px;color:#8a93a3;font-size:12px;line-height:1.55}
.label-match-list{display:flex;width:100%;flex-direction:column;gap:10px}
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
:global(.ai-analysis-drawer.el-drawer){max-width:100vw;border-radius:0!important}
:global(.ai-analysis-drawer .el-drawer__body){padding:18px 20px;background:var(--ops-surface)}
:global(.ai-analysis-drawer .el-drawer__footer){padding:12px 20px;border-top:1px solid #e5e9f2}
@media(max-width:900px){.strategy-toolbar{align-items:stretch;flex-direction:column}.strategy-toolbar>div,.strategy-toolbar .el-input{width:100%}.strategy-toolbar>div:last-child{justify-content:flex-end}.safety-grid{grid-template-columns:1fr}}
</style>

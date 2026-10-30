<template>
  <div class="settings-typography model-settings">
    <section v-loading="loading" class="model-list" aria-label="AI 模型列表">
      <button v-if="!visible" v-permission="['system-setting:edit']" type="button" class="add-model-row" @click="edit()">
        <el-icon><Plus /></el-icon><span>添加模型</span>
      </button>
      <AIModelEditor v-if="visible && !form.id" :form="form" :saving="saving" @cancel="closeEditor" @save="save" />
      <template v-for="item in models" :key="item.id">
        <article class="model-row" :class="{ 'is-active': visible && form.id === item.id }">
          <button class="model-summary" type="button" :disabled="saving || updatingIds.has(item.id)" :aria-label="`编辑 AI 模型 ${item.name}`" :aria-expanded="visible && form.id === item.id" :aria-controls="`ai-model-editor-${item.id}`" @click="toggleEditor(item)">
            <span class="provider-mark" aria-hidden="true">{{ providerMark(item.provider) }}</span>
            <span class="model-main"><strong>{{ item.name }}</strong></span>
          </button>
          <div class="row-toggle" v-permission="['system-setting:edit']">
            <el-switch :model-value="item.status === 1" :loading="updatingIds.has(item.id)" :disabled="saving" :aria-label="`${item.name}${item.status === 1 ? '停用' : '启用'}`" @change="toggleStatus(item, $event)" />
          </div>
          <el-icon class="row-arrow" aria-hidden="true"><ArrowDown /></el-icon>
        </article>
        <AIModelEditor v-if="visible && form.id === item.id" :id="`ai-model-editor-${item.id}`" :form="form" :saving="saving || updatingIds.has(item.id)" @cancel="closeEditor" @delete="remove(item)" @save="save" />
      </template>
    </section>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Plus } from '@element-plus/icons-vue'
import AIModelEditor from '../../components/AIModelEditor.vue'
import { getAIModels, saveAIModel, deleteAIModel } from '../../api/aiModel'
const models = ref([]), loading = ref(false), saving = ref(false), visible = ref(false), updatingIds = ref(new Set())
const blank = () => ({ id: null, name: '', provider: 'newapi', base_url: '', model: '', api_key: '', enabled: true })
const form = reactive(blank())
async function load() { loading.value = true; try { models.value = (await getAIModels()).data || [] } finally { loading.value = false } }
function edit(row) { if (saving.value) return; Object.assign(form, blank(), row ? { ...row, api_key: '', enabled: row.status === 1 } : {}); visible.value = true }
function closeEditor() { if (saving.value) return; visible.value = false; form.api_key = '' }
function toggleEditor(item) { if (visible.value && form.id === item.id) closeEditor(); else edit(item) }
function providerLabel(provider) { return ({ newapi: 'NewAPI', openai: 'OpenAI 兼容', anthropic: 'Anthropic' })[provider] || provider }
function providerMark(provider) { return ({ newapi: 'N', openai: 'OA', anthropic: 'A' })[provider] || 'AI' }
async function save() {
  if (saving.value || updatingIds.value.has(form.id)) return
  if (!form.name.trim() || !form.base_url.trim() || !form.model.trim() || (!form.id && !form.api_key.trim())) { ElMessage.warning('请填写完整的模型配置'); return }
  saving.value = true
  try { await saveAIModel(form.id, { name: form.name, provider: form.provider, base_url: form.base_url, model: form.model, api_key: form.api_key, status: form.enabled ? 1 : 0 }); form.api_key = ''; visible.value = false; ElMessage.success('模型已保存'); await load() } finally { saving.value = false }
}
async function toggleStatus(item, enabled) {
  if (saving.value || updatingIds.value.has(item.id)) return
  updatingIds.value.add(item.id)
  try {
    await saveAIModel(item.id, { name: item.name, provider: item.provider, base_url: item.base_url, model: item.model, api_key: '', status: enabled ? 1 : 0 })
    item.status = enabled ? 1 : 0
    if (visible.value && form.id === item.id) form.enabled = enabled
    ElMessage.success(enabled ? '模型已启用' : '模型已停用')
  } finally { updatingIds.value.delete(item.id) }
}
async function remove(row) {
  try { await ElMessageBox.confirm(`删除“${row.name}”后，使用它的会话需要重新选择模型。`, '删除模型', { type: 'warning' }); await deleteAIModel(row.id); if (visible.value && form.id === row.id) closeEditor(); await load(); ElMessage.success('模型已删除') } catch (error) { if (error !== 'cancel' && error !== 'close') ElMessage.error('删除失败') }
}
onMounted(load)
</script>

<style scoped>
.model-settings{max-width:980px;margin:0 auto}.model-list{min-height:160px}
.add-model-row{display:flex;width:100%;min-height:58px;align-items:center;gap:12px;margin:0 0 12px;padding:0 18px;color:var(--ops-primary);background:var(--ops-surface);border:1px dashed var(--ops-border);cursor:pointer;font:inherit;font-size:13px;font-weight:600;text-align:left;transition:border-color .18s ease,background-color .18s ease}
.add-model-row .el-icon{font-size:21px}.add-model-row:hover{background:color-mix(in srgb,var(--ops-primary) 4%,var(--ops-surface));border-color:var(--ops-border)}
.add-model-row:focus-visible,.model-summary:focus-visible{outline:2px solid color-mix(in srgb,var(--ops-primary) 45%,transparent);outline-offset:2px}
.model-row{position:relative;display:grid;min-height:70px;grid-template-columns:minmax(0,1fr) 44px 16px;align-items:center;gap:14px;margin-bottom:8px;padding:10px 16px;background:var(--ops-surface);border:1px solid var(--ops-border);transition:border-color .18s ease,box-shadow .18s ease}
.model-row:hover,.model-row.is-active{border-color:color-mix(in srgb,var(--ops-primary) 55%,var(--ops-border));box-shadow:0 2px 9px color-mix(in srgb,var(--ops-text) 7%,transparent)}
.model-summary{display:flex;align-items:center;gap:16px;min-width:0;padding:0;border:0;background:transparent;color:inherit;text-align:left;font:inherit;cursor:pointer}
.model-summary::after{position:absolute;inset:0;content:''}.model-summary:disabled{cursor:wait}.row-toggle{position:relative;z-index:1;display:flex;justify-content:center}
.provider-mark{display:flex;flex:0 0 36px;width:36px;height:36px;align-items:center;justify-content:center;color:var(--ops-text);background:var(--ops-surface-soft);font-size:11px;font-weight:800;border:1px solid var(--ops-border-soft);border-radius:6px}
.model-main{display:flex;flex:1;min-width:0;align-items:center;gap:20px}.model-main strong{font-weight:400;flex:0 1 180px;min-width:0;overflow:hidden;color:var(--ops-text);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.model-main>span{flex:1;min-width:0;overflow:hidden;color:var(--ops-muted);font-size:11px;text-overflow:ellipsis;white-space:nowrap}
.row-arrow{color:var(--ops-muted);transition:transform .18s ease;pointer-events:none}.model-row.is-active .row-arrow{transform:rotate(180deg)}
@media(max-width:760px){.model-row{gap:10px;padding:10px 12px}.model-summary{gap:12px}}
@media(max-width:760px){.model-main{gap:10px}.model-main strong{flex-basis:40%}}
.add-model-row,.model-row{border-radius:var(--ops-radius)}
.model-row.is-active{border-radius:var(--ops-radius) var(--ops-radius) 0 0}
</style>

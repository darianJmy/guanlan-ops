<template>
  <div class="settings-typography git-auth-page">
    <section class="account-list" v-loading="loading" aria-label="Git 认证列表">
      <button v-if="!editorVisible" type="button" class="add-account-row" @click="openEditor()" v-permission="['git-credential:edit']">
        <el-icon><Plus /></el-icon>
        <span>添加 Git 认证</span>
      </button>

      <GitCredentialEditor v-if="editorVisible && !form.id" :form="form" :rules="rules" :saving="saving" @cancel="closeEditor" @save="save" />

      <template v-for="item in accounts" :key="item.id">
      <article class="account-row" :class="{ 'is-active': form.id === item.id && editorVisible }" tabindex="0" role="button" :aria-label="`编辑 Git 认证 ${item.name}`" :aria-expanded="form.id === item.id && editorVisible" :aria-controls="`git-credential-editor-${item.id}`" @click="toggleEditor(item)" @keydown.enter.prevent="toggleEditor(item)" @keydown.space.prevent="toggleEditor(item)">
        <div class="provider-mark" :class="`is-${item.provider}`" aria-hidden="true">
          <svg v-if="item.provider === 'github'" viewBox="0 0 24 24"><path d="M11.981 2C6.47 2 2 6.469 2 11.981c0 4.41 2.86 8.152 6.826 9.472.499.092.682-.217.682-.48 0-.238-.01-1.025-.014-1.859-2.776.604-3.362-1.177-3.362-1.177-.454-1.154-1.109-1.461-1.109-1.461-.905-.62.069-.607.069-.607 1.002.07 1.53 1.029 1.53 1.029.89 1.526 2.335 1.085 2.904.83.09-.645.349-1.086.634-1.335-2.217-.253-4.548-1.108-4.548-4.933 0-1.09.39-1.98 1.029-2.68-.104-.25-.445-1.266.097-2.64 0 0 .838-.269 2.745 1.022a9.567 9.567 0 0 1 2.498-.335 9.592 9.592 0 0 1 2.501.335c1.905-1.29 2.742-1.023 2.742-1.023.543 1.375.202 2.39.098 2.642.64.699 1.027 1.59 1.027 2.679 0 3.834-2.335 4.678-4.557 4.925.358.31.677.917.677 1.848 0 1.336-.012 2.41-.012 2.74 0 .265.18.576.686.478a9.983 9.983 0 0 0 6.82-9.47C21.963 6.47 17.493 2 11.98 2" /></svg>
          <svg v-else-if="item.provider === 'gitlab'" viewBox="0 0 48 48"><path d="M42.2 22.1 25.9 5.8A2.7 2.7 0 0 0 24 5c-.7 0-1.4.3-1.9.8l-3.5 3.5 4.1 4.1a3 3 0 0 1 4 4l4 4a3 3 0 1 1-1.4 1.4l-4-4-.3.1v10.4a3 3 0 1 1-2 0V18.8a3 3 0 0 1-1.7-4.1l-4.1-4.1L5.8 22.1a2.7 2.7 0 0 0 0 3.8l16.3 16.3a2.7 2.7 0 0 0 3.8 0l16.3-16.3a2.7 2.7 0 0 0 0-3.8Z" /></svg>
          <span v-else>{{ providerMark(item.provider) }}</span>
        </div>
        <div class="account-main"><strong>{{ item.name }}</strong></div>
        <div class="row-toggle" @click.stop @keydown.stop v-permission="['git-credential:edit']">
          <el-switch :model-value="item.status === 1" :loading="updatingIds.has(item.id)" :aria-label="`${item.name}${item.status === 1 ? '停用' : '启用'}`" @change="toggleStatus(item, $event)" />
        </div>
        <el-icon class="row-arrow"><ArrowDown /></el-icon>
      </article>
      <GitCredentialEditor v-if="editorVisible && form.id === item.id" :id="`git-credential-editor-${item.id}`" :form="form" :rules="rules" :saving="saving" @cancel="closeEditor" @delete="remove(item)" @save="save" />
      </template>
    </section>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Plus } from '@element-plus/icons-vue'
import GitCredentialEditor from '../../components/GitCredentialEditor.vue'
import { createGitCredential, deleteGitCredential, getGitCredentials, updateGitCredential } from '../../api/gitCredential'

const loading = ref(false), saving = ref(false), editorVisible = ref(false), accounts = ref([]), updatingIds = ref(new Set())
const blankForm = () => ({ id:null, name:'', provider:'gitlab', base_url:'', auth_type:'none', username:'', secret:'', secret_configured:false, description:'', enabled:true })
const form = reactive(blankForm())
const rules = {
  name:[{ required:true, message:'请输入认证名称', trigger:'blur' }],
  provider:[{ required:true, message:'请选择 Git 服务', trigger:'change' }],
  base_url:[{ required:true, message:'请输入 Git 服务地址', trigger:'blur' }],
  auth_type:[{ required:true, message:'请选择认证方式', trigger:'change' }],
}
onMounted(load)
async function load() { loading.value=true; try { accounts.value=(await getGitCredentials()).data||[] } finally { loading.value=false } }
function openEditor(item) { Object.assign(form, blankForm(), item ? { ...item, secret:'', enabled:item.status===1 } : {}); editorVisible.value=true }
function toggleEditor(item) { if (editorVisible.value && form.id === item.id) return closeEditor(); openEditor(item) }
function closeEditor() { editorVisible.value=false }
async function save() {
  if (form.auth_type !== 'none' && !form.secret_configured && !form.secret.trim()) return ElMessage.warning(form.auth_type === 'ssh_key' ? '请输入 SSH 私钥' : '请输入 Token 或密码')
  saving.value=true
  try {
    const payload={ name:form.name.trim(), provider:form.provider, base_url:form.base_url.trim(), auth_type:form.auth_type, username:form.username.trim(), secret:form.secret, description:form.description.trim(), status:form.enabled?1:0 }
    if(form.id) await updateGitCredential(form.id,payload); else await createGitCredential(payload)
    closeEditor(); await load(); ElMessage.success('Git 认证已保存')
  } finally { saving.value=false }
}
async function toggleStatus(item, enabled) {
  if (updatingIds.value.has(item.id)) return
  updatingIds.value.add(item.id)
  try {
    await updateGitCredential(item.id,{ name:item.name, provider:item.provider, base_url:item.base_url, auth_type:item.auth_type, username:item.username||'', secret:'', description:item.description||'', status:enabled?1:0 })
    item.status=enabled?1:0
    if (editorVisible.value && form.id === item.id) form.enabled=enabled
    ElMessage.success(enabled?'Git 认证已启用':'Git 认证已停用')
  } finally { updatingIds.value.delete(item.id) }
}
async function remove(item) { await ElMessageBox.confirm(`确认删除 Git 认证「${item.name}」？`,'删除确认',{type:'warning'}); await deleteGitCredential(item.id); if(editorVisible.value&&form.id===item.id)closeEditor(); await load(); ElMessage.success('Git 认证已删除') }
function providerMark(provider) { return ({github:'GH',gitlab:'GL',gitee:'GE'})[provider]||'GIT' }
</script>

<style scoped>
.git-auth-page{--ink:var(--ops-text);--muted:var(--ops-muted);--border:var(--ops-border);max-width:980px;margin:0 auto}.account-list{min-height:160px}.add-account-row{display:flex;width:100%;min-height:58px;align-items:center;gap:12px;margin:0 0 12px;padding:0 18px;color:var(--ops-primary);background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 55%,var(--border));cursor:pointer;font:inherit;font-size:13px;font-weight:600;text-align:left;transition:border-color .18s ease,background-color .18s ease}.add-account-row .el-icon{font-size:21px}.add-account-row:hover{background:color-mix(in srgb,var(--ops-primary) 4%,var(--ops-surface));border-color:var(--ops-primary)}.add-account-row:focus-visible,.account-row:focus-visible{outline:2px solid color-mix(in srgb,var(--ops-primary) 45%,transparent);outline-offset:2px}.account-editor{margin-bottom:12px;background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 45%,var(--border))}.account-editor.is-editing{border-style:solid;border-color:var(--ops-primary)}.editor-heading{display:flex;align-items:center;justify-content:space-between;padding:16px 18px;border-bottom:1px solid var(--ops-border-soft)}.editor-heading>div{display:flex;flex-direction:column;gap:4px}.editor-heading strong{color:var(--ink);font-size:15px}.editor-heading span{color:var(--muted);font-size:11px}.account-editor :deep(.el-form){padding:20px 22px}.two-column{display:grid;grid-template-columns:1fr 2fr;gap:18px}.auth-type-group{display:grid;width:100%;grid-template-columns:repeat(3,1fr)}.auth-type-group :deep(.el-radio-button),.auth-type-group :deep(.el-radio-button__inner){width:100%}.auth-note{display:flex;align-items:flex-start;gap:8px;margin:-4px 0 18px;padding:9px 11px;color:var(--ops-muted);background:color-mix(in srgb,var(--ops-primary) 6%,transparent);font-size:11px;line-height:1.55}.auth-note .el-icon{margin-top:2px;flex:none}.editor-footer{display:flex;align-items:center;justify-content:space-between;padding-top:16px;border-top:1px solid var(--ops-border-soft)}.status-control{display:flex;align-items:center;gap:14px}.status-control>span{display:flex;flex-direction:column;gap:2px}.status-control strong{color:var(--ops-text);font-size:12px}.status-control small{color:var(--muted);font-size:10px}.account-row{display:grid;min-height:70px;grid-template-columns:38px minmax(220px,1fr) minmax(180px,auto) 64px 52px 16px;align-items:center;gap:14px;margin-bottom:8px;padding:10px 16px;cursor:pointer;background:var(--ops-surface);border:1px solid var(--border);transition:border-color .18s ease,box-shadow .18s ease}.account-row:hover,.account-row.is-active{border-color:color-mix(in srgb,var(--ops-primary) 55%,var(--border));box-shadow:0 2px 9px color-mix(in srgb,var(--ops-text) 7%,transparent)}.provider-mark{display:flex;width:36px;height:36px;align-items:center;justify-content:center;color:var(--ops-text);background:var(--ops-surface-soft);font-size:9px;font-weight:800;border:1px solid var(--ops-border-soft);border-radius:6px}.provider-mark svg{width:22px;height:22px;fill:currentColor}.provider-mark.is-github{color:var(--ops-text)}.provider-mark.is-gitlab{color:#f4511e}.provider-mark.is-gitee{color:#c71d23}.account-main{display:flex;flex:1;min-width:0;align-items:center;gap:20px}.account-main strong{font-weight:400;flex:0 1 180px;min-width:0;overflow:hidden;color:var(--ink);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.account-main span{flex:1;min-width:0;overflow:hidden;color:var(--muted);font-size:11px;text-overflow:ellipsis;white-space:nowrap}.account-meta{display:flex;align-items:center;justify-content:flex-end;gap:10px}.account-meta>span{color:var(--muted);font-size:10px}.account-status{display:flex;align-items:center;gap:5px;color:var(--muted);font-size:10px;white-space:nowrap}.account-status i{width:6px;height:6px;background:#a5adb8;border-radius:50%}.account-status.is-enabled{color:#23855a}.account-status.is-enabled i{background:#2aae72}.row-actions{display:flex;justify-content:flex-end}.row-arrow{color:var(--muted);transition:transform .18s ease}.account-row.is-active .row-arrow{transform:rotate(180deg)}:deep(.el-select){width:100%}@media(max-width:760px){.two-column{grid-template-columns:1fr}.account-row{grid-template-columns:38px minmax(0,1fr) auto 16px}.account-meta{grid-column:2;justify-content:flex-start}.account-status{grid-column:3;grid-row:1}.row-actions{grid-column:3;grid-row:2}.row-arrow{grid-column:4;grid-row:1/3}.editor-footer{align-items:flex-end;gap:14px}}
.account-row{grid-template-columns:38px minmax(0,1fr) 44px 16px}.row-toggle{display:flex;justify-content:center}@media(max-width:760px){.row-toggle{grid-column:3;grid-row:1/3}}
@media(max-width:760px){.account-main{gap:10px}.account-main strong{flex-basis:40%}}
.auth-note .el-icon,.config-note .el-icon{color:var(--ops-primary)}
</style>

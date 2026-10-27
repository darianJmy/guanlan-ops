<template>
  <div class="settings-typography token-page">
    <section class="token-list" v-loading="loading" aria-label="访问令牌列表">
      <button v-if="!createVisible && !detailVisible" type="button" class="add-token-row" @click="openCreate" v-permission="['api-token:edit']">
        <el-icon><Plus /></el-icon><span>添加访问令牌</span>
      </button>

      <article v-if="createVisible" class="token-editor is-creating">
        <header class="editor-heading"><strong>添加访问令牌</strong></header>
        <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
          <div class="form-grid">
            <el-form-item label="Token 名称" prop="name"><el-input v-model="form.name" maxlength="100" /></el-form-item>
            <el-form-item label="绑定用户" prop="user_id"><el-select v-model="form.user_id" filterable placeholder="选择 Token 使用的身份"><el-option v-for="user in users" :key="user.id" :label="user.username" :value="user.id" /></el-select></el-form-item>
          </div>
          <el-form-item label="有效期" prop="expires_in_days">
            <el-radio-group v-model="form.expires_in_days" class="expiry-options"><el-radio-button v-for="option in expiryOptions" :key="option.value" :value="option.value">{{ option.label }}</el-radio-button></el-radio-group>
          </el-form-item>
          <div class="auth-note" role="note" aria-label="访问令牌说明">
            <el-icon aria-hidden="true"><InfoFilled /></el-icon>
            <span>访问令牌用于脚本或第三方系统调用平台 API，请在请求头中携带 <code>Authorization: Bearer &lt;Token&gt;</code>。令牌继承绑定用户的角色权限，用户停用或令牌禁用后立即失效。</span>
          </div>
          <el-form-item label="备注"><el-input v-model="form.description" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
          <footer class="editor-footer is-create"><span></span><div><el-button @click="closeEditor">取消</el-button><el-button type="primary" :loading="creating" @click="create">创建</el-button></div></footer>
        </el-form>
      </article>

      <template v-for="item in tokens" :key="item.id">
        <article class="token-row" :class="{ 'is-active': detailVisible && detailToken?.id === item.id, 'is-disabled': item.status !== 1 || isExpired(item) }" tabindex="0" role="button" :aria-label="`编辑访问令牌 ${item.name}`" :aria-expanded="detailVisible && detailToken?.id === item.id" :aria-controls="`token-editor-${item.id}`" @click="toggleEditor(item)" @keydown.enter.prevent="toggleEditor(item)" @keydown.space.prevent="toggleEditor(item)">
          <div class="token-mark" aria-hidden="true"><el-icon><Key /></el-icon></div>
          <el-tooltip :disabled="!item.description" :content="item.description" placement="top-start"><strong class="token-name">{{ item.name }}</strong></el-tooltip>
          <div class="token-meta">
            <el-tooltip content="绑定用户" placement="top"><span><el-icon><User /></el-icon>{{ item.username }}</span></el-tooltip>
            <el-tooltip content="有效期" placement="top"><span><el-icon><Calendar /></el-icon>{{ item.permanent ? '永久' : formatDate(item.expires_at) }}</span></el-tooltip>
          </div>
          <div class="row-toggle" @click.stop @keydown.stop v-permission="['api-token:edit']"><el-switch :model-value="item.status === 1" :disabled="isExpired(item)" :aria-label="`${item.name}${item.status === 1 ? '停用' : '启用'}`" @change="changeStatus(item, $event)" /></div>
          <el-icon class="row-arrow"><ArrowDown /></el-icon>
        </article>

        <article v-if="detailVisible && detailToken?.id === item.id" :id="`token-editor-${item.id}`" class="token-editor is-editing">
          <el-descriptions :column="2" border class="token-detail-meta">
            <el-descriptions-item label="绑定用户">{{ item.username }}</el-descriptions-item>
            <el-descriptions-item label="状态">{{ tokenStatus(item) }}</el-descriptions-item>
            <el-descriptions-item label="有效期至">{{ item.permanent ? '永久' : formatTime(item.expires_at) }}</el-descriptions-item>
            <el-descriptions-item label="最后使用"><el-tooltip :disabled="!item.last_used_ip" :content="`来源 IP：${item.last_used_ip}`" placement="top-start"><span>{{ item.last_used_at ? formatTime(item.last_used_at) : '从未使用' }}</span></el-tooltip></el-descriptions-item>
          </el-descriptions>
          <el-form label-position="top">
            <div class="auth-note" role="note" aria-label="访问令牌说明">
            <el-icon aria-hidden="true"><InfoFilled /></el-icon>
            <span>访问令牌用于脚本或第三方系统调用平台 API，请在请求头中携带 <code>Authorization: Bearer &lt;Token&gt;</code>。令牌继承绑定用户的角色权限，用户停用或令牌禁用后立即失效。</span>
          </div>
          <el-form-item label="备注"><el-input v-model="detailDescription" type="textarea" :rows="3" maxlength="500" show-word-limit /></el-form-item>
            <section class="regenerate-panel">
              <div class="regenerate-title"><strong>重新生成 Token</strong><el-tooltip content="重新生成后，当前 Token 会立即失效。" placement="top"><el-icon class="help-icon" aria-label="重新生成说明"><InfoFilled /></el-icon></el-tooltip></div>
              <div class="regenerate-control">
                <el-select v-model="regenerateExpiresInDays" aria-label="新 Token 有效期"><el-option v-for="option in expiryOptions" :key="option.value" :label="option.label" :value="option.value" /></el-select>
                <el-button :icon="RefreshRight" :loading="regenerating" @click="regenerate">重新生成</el-button>
              </div>
            </section>
            <footer class="editor-footer"><div><el-button type="danger" plain @click="remove(item)">删除</el-button></div><div><el-button @click="closeEditor">取消</el-button><el-button type="primary" :loading="updating" @click="saveDescription">保存</el-button></div></footer>
          </el-form>
        </article>
      </template>
    </section>

    <el-dialog v-model="revealVisible" title="Token 生成成功" width="620px" :close-on-click-modal="false" :close-on-press-escape="false" @closed="revealedToken=''">
      <div class="one-time-warning"><el-icon><WarningFilled /></el-icon><span><strong>请立即复制并妥善保存</strong>关闭后平台不会再次显示完整 Token，遗失后只能重新生成。</span></div>
      <label class="token-label">API Token</label>
      <el-input :model-value="revealedToken" readonly class="revealed-token"><template #append><el-button :icon="CopyDocument" @click="copyToken">复制</el-button></template></el-input>
      <template #footer><el-button type="primary" @click="revealVisible=false">我已保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { ArrowDown, Calendar, CopyDocument, InfoFilled, Key, Plus, RefreshRight, User, WarningFilled } from '@element-plus/icons-vue'
import { createAPIToken, deleteAPIToken, getAPITokens, regenerateAPIToken, updateAPIToken, updateAPITokenStatus } from '../../api/apiToken'
import { getUsers } from '../../api/user'

const loading=ref(false),creating=ref(false),updating=ref(false),regenerating=ref(false),createVisible=ref(false),detailVisible=ref(false),revealVisible=ref(false),tokens=ref([]),users=ref([]),formRef=ref(),revealedToken=ref(''),detailToken=ref(null),detailDescription=ref(''),regenerateExpiresInDays=ref(90)
const expiryOptions=[{label:'7 天',value:7},{label:'30 天',value:30},{label:'60 天',value:60},{label:'90 天',value:90},{label:'永久',value:0}]
const blankForm=()=>({name:'',user_id:null,expires_in_days:90,description:''})
const form=reactive(blankForm())
const rules={name:[{required:true,message:'请输入 Token 名称',trigger:'blur'}],user_id:[{required:true,message:'请选择绑定用户',trigger:'change'}],expires_in_days:[{required:true,message:'请选择有效期',trigger:'change'}]}

onMounted(load)
async function load(){loading.value=true;try{const [tokenResult,userResult]=await Promise.all([getAPITokens(),getUsers({page:1,page_size:1000})]);tokens.value=tokenResult.data||[];users.value=(userResult.data?.list||[]).filter(item=>item.status===1)}finally{loading.value=false}}
function openCreate(){closeEditor();Object.assign(form,blankForm());createVisible.value=true}
function openDetail(row){createVisible.value=false;detailToken.value=row;detailDescription.value=row.description||'';regenerateExpiresInDays.value=90;detailVisible.value=true}
function toggleEditor(row){if(detailVisible.value&&detailToken.value?.id===row.id)return closeEditor();openDetail(row)}
function closeEditor(){createVisible.value=false;detailVisible.value=false;detailToken.value=null;formRef.value?.clearValidate()}
async function create(){if(!await formRef.value.validate().catch(()=>false))return;creating.value=true;try{const result=await createAPIToken({name:form.name.trim(),user_id:form.user_id,expires_in_days:form.expires_in_days,description:form.description.trim()});revealedToken.value=result.data.token;createVisible.value=false;revealVisible.value=true;await load()}finally{creating.value=false}}
async function saveDescription(){if(!detailToken.value)return;updating.value=true;try{await updateAPIToken(detailToken.value.id,{description:detailDescription.value.trim()});await load();closeEditor();ElMessage.success('访问令牌备注已保存')}finally{updating.value=false}}
async function regenerate(){if(!detailToken.value)return;await ElMessageBox.confirm('重新生成后，旧 Token 会立即失效。确认继续？','重新生成访问令牌',{type:'warning'});regenerating.value=true;try{const result=await regenerateAPIToken(detailToken.value.id,regenerateExpiresInDays.value);revealedToken.value=result.data.token;closeEditor();revealVisible.value=true;await load()}finally{regenerating.value=false}}
async function changeStatus(row,value){await updateAPITokenStatus(row.id,value?1:0);await load();ElMessage.success(value?'访问令牌已启用':'访问令牌已禁用')}
async function remove(row){await ElMessageBox.confirm(`删除后使用「${row.name}」的调用会立即失败，确认删除？`,'删除访问令牌',{type:'warning'});await deleteAPIToken(row.id);closeEditor();await load();ElMessage.success('访问令牌已删除')}
async function copyToken(){try{await navigator.clipboard.writeText(revealedToken.value);ElMessage.success('Token 已复制')}catch{ElMessage.error('复制失败，请手动复制')}}
function isExpired(row){return !row.permanent&&new Date(row.expires_at).getTime()<=Date.now()}
function tokenStatus(row){if(isExpired(row))return'已过期';return row.status===1?'启用':'禁用'}
function formatDate(value){return new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit'}).format(new Date(value))}
function formatTime(value){return new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false}).format(new Date(value))}
</script>

<style scoped>
.token-page{--ink:var(--ops-text);--muted:var(--ops-muted);--border:var(--ops-border);max-width:980px;margin:0 auto}.auth-note{display:flex;align-items:flex-start;gap:8px;margin:-4px 0 18px;padding:9px 11px;color:var(--ops-muted);background:color-mix(in srgb,var(--ops-primary) 6%,transparent);font-size:11px;line-height:1.55}.auth-note .el-icon{margin-top:2px;flex:none}.auth-note span{min-width:0;overflow-wrap:anywhere}.auth-note code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}.token-list{min-height:160px}.add-token-row{display:flex;width:100%;min-height:58px;align-items:center;gap:12px;margin:0 0 12px;padding:0 18px;color:var(--ops-primary);background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 55%,var(--border));cursor:pointer;font:inherit;font-size:13px;font-weight:600;text-align:left;transition:border-color .18s ease,background-color .18s ease}.add-token-row .el-icon{font-size:21px}.add-token-row:hover{background:color-mix(in srgb,var(--ops-primary) 4%,var(--ops-surface));border-color:var(--ops-primary)}.add-token-row:focus-visible,.token-row:focus-visible{outline:2px solid color-mix(in srgb,var(--ops-primary) 45%,transparent);outline-offset:2px}.token-row{display:grid;min-height:70px;grid-template-columns:38px minmax(180px,1fr) minmax(260px,auto) 52px 16px;align-items:center;gap:14px;margin-bottom:8px;padding:10px 16px;cursor:pointer;background:var(--ops-surface);border:1px solid var(--border);transition:border-color .18s ease,box-shadow .18s ease}.token-row:hover,.token-row.is-active{border-color:color-mix(in srgb,var(--ops-primary) 55%,var(--border));box-shadow:0 2px 9px color-mix(in srgb,var(--ops-text) 7%,transparent)}.token-row.is-disabled .token-mark{color:var(--muted)}.token-mark{display:flex;width:36px;height:36px;align-items:center;justify-content:center;color:var(--ops-primary);background:var(--ops-surface-soft);border:1px solid var(--ops-border-soft);border-radius:6px}.token-mark .el-icon{font-size:19px}.token-name{font-weight:400;min-width:0;overflow:hidden;color:var(--ink);font-size:13px;text-overflow:ellipsis;white-space:nowrap}.token-meta{display:flex;align-items:center;justify-content:flex-end;gap:20px}.token-meta span{display:inline-flex;align-items:center;gap:6px;color:var(--muted);font-size:11px;white-space:nowrap}.token-meta .el-icon{font-size:13px}.row-toggle{display:flex;justify-content:center}.row-arrow{color:var(--muted);transition:transform .18s ease}.token-row.is-active .row-arrow{transform:rotate(180deg)}.token-editor{margin-bottom:12px;background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 45%,var(--border))}.token-editor.is-editing{border-style:solid;border-color:var(--ops-primary)}.editor-heading{padding:16px 18px;border-bottom:1px solid var(--ops-border-soft)}.editor-heading strong{color:var(--ink);font-size:15px}.token-editor>form,.token-editor>.el-form{padding:20px 22px}.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:18px}.expiry-options{display:grid;width:100%;grid-template-columns:repeat(5,minmax(0,1fr))}.expiry-options :deep(.el-radio-button),.expiry-options :deep(.el-radio-button__inner){width:100%}.token-detail-meta{margin:20px 22px 0}.regenerate-panel{margin-top:4px;padding:16px;background:var(--ops-surface-soft);border:1px solid var(--ops-border-soft)}.regenerate-title{display:flex;align-items:center;gap:6px;margin-bottom:14px}.regenerate-title strong{color:var(--ops-text);font-size:13px}.help-icon{color:var(--ops-muted);cursor:help;font-size:14px}.regenerate-control{display:flex;align-items:center;gap:10px}.regenerate-control .el-select{min-width:0;flex:1}.regenerate-control .el-button{flex:none}.editor-footer{display:flex;align-items:center;justify-content:space-between;margin-top:20px;padding-top:16px;border-top:1px solid var(--ops-border-soft)}.editor-footer.is-create>span{display:block}.one-time-warning{display:flex;gap:10px;margin-bottom:20px;padding:12px 14px;color:#8a6520;background:#fff8e6;border:1px solid #ead7a7}.one-time-warning>.el-icon{margin-top:2px;flex:none}.one-time-warning span{display:flex;flex-direction:column;gap:4px;font-size:11px;line-height:1.5}.one-time-warning strong{font-size:12px}.token-label{display:block;margin-bottom:8px;color:var(--ops-text);font-size:12px;font-weight:650}.revealed-token :deep(.el-input__inner){font-family:ui-monospace,SFMono-Regular,Menlo,monospace}:deep(.el-select){width:100%}@media(max-width:760px){.token-row{grid-template-columns:38px minmax(0,1fr) 52px 16px}.token-meta{grid-column:2;justify-content:flex-start;flex-wrap:wrap;gap:8px 14px}.row-toggle{grid-column:3;grid-row:1/3}.row-arrow{grid-column:4;grid-row:1/3}.form-grid{grid-template-columns:1fr}.expiry-options{grid-template-columns:repeat(3,minmax(0,1fr))}}
.auth-note .el-icon,.config-note .el-icon{color:var(--ops-primary)}
</style>

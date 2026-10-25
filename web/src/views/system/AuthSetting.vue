<template>
  <div class="auth-settings" v-loading="loading">
    <section class="provider-selector-card">
      <div class="selector-title"><strong>外部认证</strong><span>系统账号始终可用；LDAP 与 OIDC 最多启用一种</span></div>
      <div class="provider-selector" role="tablist" aria-label="外部认证方式">
        <button type="button" role="tab" class="provider-option" :class="{ 'is-active': activeProvider === 'none' }" :aria-selected="activeProvider === 'none'" @click="selectProvider('none')">
          <span class="provider-icon none">—</span>
          <span class="provider-option-copy"><b>无</b><small>不启用外部认证</small></span>
          <span class="provider-status" :class="{ 'is-enabled': activeProvider === 'none' }"><i></i>{{ activeProvider === 'none' ? '当前方式' : '未选择' }}</span>
        </button>
        <button type="button" role="tab" class="provider-option" :class="{ 'is-active': activeProvider === 'ldap' }" :aria-selected="activeProvider === 'ldap'" @click="selectProvider('ldap')">
          <span class="provider-icon ldap">
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M13.636468,8.13419742 C13.6525132,8.13954797 14.9343445,10.3546748 16.2121645,12.5738124 L16.5181142,13.1053531 C17.6856416,15.1345625 18.7637933,17.0178099 18.7637933,17.037492 C18.7637933,17.0593831 15.8972691,17.0917742 13.053894,17.105656 L12.5378151,17.1079695 C12.0232778,17.1100661 11.5152952,17.1114928 11.0312358,17.1120751 L10.5557941,17.1123694 C9.28287672,17.1123694 8.45922432,17.0981071 8.46630957,17.0802791 C8.47316287,17.0608823 8.60963149,16.8184903 8.83415071,16.4249873 L9.01840017,16.10265 C9.45056361,15.3477308 10.0877965,14.2412391 10.7517439,13.091631 L11.0594286,12.5591043 C12.3439399,10.3368468 13.6257712,8.13063399 13.636468,8.13419742 Z M6.20220281,6.85414372 C6.21646519,6.85058761 6.80835393,7.85251975 7.5143417,9.08264996 C7.60259017,9.23597053 7.69267715,9.39257814 7.78334912,9.55027214 L8.05586641,10.0244127 C8.59939677,10.9704096 9.11885936,11.8762935 9.34349183,12.2667261 L9.88902784,13.2187399 L9.14381853,14.5130508 L8.39504362,15.8109273 L4.69752181,15.8216241 L1,15.8287553 L1.11053344,15.6433444 C1.41208088,15.1176738 1.73087417,14.5629692 2.05590464,13.998233 L2.3357729,13.5121599 C4.20832101,10.2612042 6.18182799,6.85997284 6.20220281,6.85414372 Z M17.8296075,6.84345642 C17.8427289,6.84345642 18.934002,8.71758439 20.110486,10.7492779 L20.418675,11.2817305 C21.705409,13.5057703 22.9908059,15.7396154 22.9997199,15.778837 C23.0086339,15.8144929 22.8095549,15.8253878 21.7132995,15.828029 L18.8707611,15.8287553 L18.8208428,15.7503122 C18.660391,15.4935894 15.7686937,10.4411416 15.7686937,10.4126168 C15.7686937,10.3662641 17.8046483,6.84345642 17.8296075,6.84345642 Z M9.83910951,5.00002191 C9.8457489,4.99781333 9.9486975,5.16273312 10.1084506,5.42732976 L10.2856754,5.7226538 C10.9619921,6.85509018 12.1728528,8.93314088 12.2244924,9.06482198 C12.2387548,9.10047793 11.9071545,9.70662905 11.3259625,10.719258 C10.8196481,11.5963943 10.3810799,12.3629972 10.345424,12.4200467 L10.2848089,12.5198833 L10.195669,12.3843907 C10.1141521,12.2533244 9.6421369,11.4351243 9.14610621,10.5696097 L8.91679822,10.1691116 C8.38399494,9.23764946 7.88872916,8.36547359 7.88872916,8.35170302 C7.88872916,8.2982191 9.79988797,5.01074069 9.83910951,5.00002191 Z" />
            </svg>
          </span>
          <span class="provider-option-copy"><b>LDAP</b><small>企业目录账号</small></span>
          <span class="provider-status" :class="{ 'is-enabled': form.ldap_enabled }"><i></i>{{ form.ldap_enabled ? '已启用' : '未启用' }}</span>
        </button>
        <button type="button" role="tab" class="provider-option" :class="{ 'is-active': activeProvider === 'oidc' }" :aria-selected="activeProvider === 'oidc'" @click="selectProvider('oidc')">
          <span class="provider-icon oidc">
            <svg viewBox="0 0 20 18.55" aria-hidden="true">
              <path class="oidc-orange" d="M9,16.979V1.985L9,.104l3,1.412V18.445Z" />
              <path class="oidc-gray" d="M9,.104V1.985C5.576,2.414,3,4.285,3,6.527c0,2.126,2.316,3.916,5.473,4.462v1.906C3.644,12.311,0,9.682,0,6.527,0,3.258,3.912.554,9,.104ZM19.804,7.73l-.414,4.3-1.543-.869-.217.13a11.928,11.928,0,0,1-5.1,1.6V10.989a9.256,9.256,0,0,0,3.1-1.08L14,8.992Z" />
            </svg>
          </span>
          <span class="provider-option-copy"><b>OIDC</b><small>单点登录</small></span>
          <span class="provider-status" :class="{ 'is-enabled': form.oidc_enabled }"><i></i>{{ form.oidc_enabled ? '已启用' : '未启用' }}</span>
        </button>
      </div>

      <div v-if="activeProvider !== 'none'" class="provider-detail-card">
      <template v-if="activeProvider === 'ldap'">
        <div class="editor-shell">
          <div class="editor-toolbar" role="tablist" aria-label="LDAP 配置编辑器">
            <button type="button" role="tab" :class="{ active: ldapEditorTab === 'configuration' }" :aria-selected="ldapEditorTab === 'configuration'" @click="ldapEditorTab = 'configuration'">配置</button>
            <button type="button" role="tab" :class="{ active: ldapEditorTab === 'sample' }" :aria-selected="ldapEditorTab === 'sample'" @click="ldapEditorTab = 'sample'">示例</button>
            <span>YAML</span>
          </div>
          <CodeEditor v-if="ldapEditorTab === 'configuration'" v-model="form.ldap_config" language="yaml" height="340px" aria-label="LDAP YAML 配置" />
          <pre v-else class="sample-code">{{ ldapSample }}</pre>
        </div>
        <div class="config-note"><el-icon><InfoFilled /></el-icon><span><code>bind_password</code> 留空时沿用已保存的密码；填写新值后保存，页面不会再次回显明文。</span></div>
      </template>

      <template v-else>
        <div class="editor-shell">
          <div class="editor-toolbar" role="tablist" aria-label="OIDC 配置编辑器">
            <button type="button" role="tab" :class="{ active: oidcEditorTab === 'configuration' }" :aria-selected="oidcEditorTab === 'configuration'" @click="oidcEditorTab = 'configuration'">配置</button>
            <button type="button" role="tab" :class="{ active: oidcEditorTab === 'sample' }" :aria-selected="oidcEditorTab === 'sample'" @click="oidcEditorTab = 'sample'">示例</button>
            <span>YAML</span>
          </div>
          <CodeEditor v-if="oidcEditorTab === 'configuration'" v-model="form.oidc_config" language="yaml" height="340px" aria-label="OIDC YAML 配置" />
          <pre v-else class="sample-code">{{ oidcSample }}</pre>
        </div>
        <div class="config-note"><el-icon><InfoFilled /></el-icon><span><code>client_secret</code> 留空时沿用已保存的密钥；填写新值后保存，页面不会再次回显明文。</span></div>
      </template>
      </div>

      <div v-if="activeProvider !== 'none'" class="permission-setting">
        <div class="section-title">
          <strong class="tooltip-title">
            外部用户默认权限
            <el-tooltip content="LDAP 或 OIDC 用户第一次登录时自动绑定此角色" placement="top">
              <el-icon class="help-icon" tabindex="0" aria-label="外部用户默认权限说明"><QuestionFilled /></el-icon>
            </el-tooltip>
          </strong>
        </div>
        <el-select v-model="form.default_role_id" clearable placeholder="不自动分配角色"><el-option v-for="role in roles" :key="role.id" :label="role.name" :value="role.id" /></el-select>
      </div>

      <footer class="settings-actions">
        <el-button type="primary" :loading="saving" @click="save" v-permission="['system-setting:edit']">保存设置</el-button>
      </footer>
    </section>
  </div>
</template>

<script setup>
import { defineAsyncComponent, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { InfoFilled, QuestionFilled } from '@element-plus/icons-vue'
import { getAuthSettings, updateAuthSettings } from '../../api/systemSetting'
import { getRoles } from '../../api/role'

// Keep Monaco out of the route's initial dependency chain, especially when external auth is disabled.
const CodeEditor = defineAsyncComponent(() => import('../../components/CodeEditor.vue'))

const ldapSample=`connection:
  url: ldaps://ldap.example.com:636
  bind_dn: cn=reader,dc=example,dc=com
  bind_password: ""

user_search:
  base_dn: dc=example,dc=com
  filter: (uid={username})

attributes:
  username: uid
  display_name: cn
  email: mail`
const oidcSample=`connection:
  issuer_url: https://accounts.example.com
  client_id: aiops
  client_secret: ""
  redirect_url: https://aiops.example.com/api/auth/oidc/callback

authorization:
  scopes:
    - openid
    - profile
    - email

claims:
  username: preferred_username`
const loading=ref(false),saving=ref(false),roles=ref([]),activeProvider=ref('none'),ldapEditorTab=ref('configuration'),oidcEditorTab=ref('configuration')
const form=reactive({local_enabled:true,ldap_enabled:false,ldap_url:'',ldap_base_dn:'',ldap_bind_dn:'',ldap_bind_password:'',ldap_bind_password_configured:false,ldap_user_filter:'(uid={username})',ldap_username_attr:'uid',ldap_display_name_attr:'cn',ldap_email_attr:'mail',ldap_config:'',oidc_enabled:false,oidc_issuer_url:'',oidc_client_id:'',oidc_client_secret:'',oidc_client_secret_configured:false,oidc_redirect_url:'',oidc_scopes:'openid profile email',oidc_username_claim:'preferred_username',oidc_config:'',default_role_id:null})
onMounted(load)
async function load(){loading.value=true;try{const [auth,roleResult]=await Promise.all([getAuthSettings(),getRoles()]);Object.assign(form,auth.data||{});form.default_role_id=form.default_role_id||null;roles.value=(roleResult.data||[]).filter(item=>item.status===1);activeProvider.value=form.ldap_enabled?'ldap':form.oidc_enabled?'oidc':'none';if(!form.ldap_config)form.ldap_config=ldapSample;if(!form.oidc_issuer_url&&!form.oidc_client_id&&!form.oidc_redirect_url)form.oidc_config=oidcSample.replace('https://aiops.example.com',location.origin)}finally{loading.value=false}}
function payload(overrides={}){return{...form,local_enabled:true,ldap_enabled:activeProvider.value==='ldap',oidc_enabled:activeProvider.value==='oidc',ldap_bind_password:'',oidc_client_secret:'',default_role_id:Number(form.default_role_id||0),...overrides}}
function selectProvider(provider){activeProvider.value=provider;form.ldap_enabled=provider==='ldap';form.oidc_enabled=provider==='oidc'}
async function save(){saving.value=true;try{await updateAuthSettings(payload());form.oidc_client_secret='';await load();ElMessage.success('登录认证配置已保存')}finally{saving.value=false}}
</script>

<style scoped>
.auth-settings{display:flex;max-width:980px;margin:0 auto;flex-direction:column;gap:14px}.settings-header{display:flex;align-items:flex-start;justify-content:space-between;gap:20px;margin-bottom:4px}.settings-header h1{margin:0;color:var(--ops-text);font-size:21px}.settings-header p{margin:6px 0 0;color:var(--ops-muted);font-size:12px}.provider-card,.provider-selector-card{padding:20px 22px;background:var(--ops-surface);border:1px solid var(--ops-border)}.selector-title,.section-title{display:flex;flex-direction:column;gap:4px;margin-bottom:14px}.selector-title strong,.section-title strong,.provider-header strong{color:var(--ops-text);font-size:14px}.selector-title span,.section-title span,.provider-header span{color:var(--ops-muted);font-size:11px}.provider-selector{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.provider-option{position:relative;display:grid;grid-template-columns:42px minmax(0,1fr);grid-template-rows:auto auto;align-items:center;gap:2px 12px;min-height:78px;padding:14px 15px;overflow:hidden;cursor:pointer;background:var(--ops-surface-soft);border:1px solid var(--ops-border);transition:border-color .18s ease,box-shadow .18s ease,background-color .18s ease}.provider-option:hover{border-color:color-mix(in srgb,var(--ops-primary) 55%,var(--ops-border))}.provider-option.is-active{background:color-mix(in srgb,var(--ops-primary) 5%,var(--ops-surface));border-color:var(--ops-primary);box-shadow:0 0 0 1px color-mix(in srgb,var(--ops-primary) 28%,transparent)}.provider-option input{position:absolute;width:1px;height:1px;opacity:0;pointer-events:none}.provider-option:focus-within{outline:2px solid color-mix(in srgb,var(--ops-primary) 45%,transparent);outline-offset:2px}.provider-icon{display:grid;grid-row:1/3;width:40px;height:40px;place-items:center;color:#fff;font-size:18px;border-radius:8px}.provider-icon.local{background:#2d6dcc}.provider-icon.ldap,.provider-icon.oidc{background:var(--ops-surface);border:1px solid var(--ops-border-soft)}.provider-icon svg{width:24px;height:24px}.provider-icon.ldap svg{fill:#cb2026}.provider-icon.oidc svg{width:25px}.oidc-orange{fill:#f8931e}.oidc-gray{fill:#9da4ad}.provider-option-copy{display:flex;align-self:end;min-width:0;flex-direction:column;gap:2px}.provider-option-copy b{color:var(--ops-text);font-size:14px;line-height:18px}.provider-option-copy small{color:var(--ops-muted);font-size:11px;line-height:16px}.provider-status{display:flex;align-self:start;align-items:center;gap:5px;color:var(--ops-muted);font-size:10px}.provider-status i{width:6px;height:6px;background:#a5adb8;border-radius:50%}.provider-status.is-enabled{color:#23855a}.provider-status.is-enabled i{background:#2aae72}.provider-detail-card{min-height:116px}.provider-header{display:grid;grid-template-columns:minmax(0,1fr) auto;align-items:center;gap:13px}.provider-header>div{display:flex;flex-direction:column;gap:4px}.safety-note,.disabled-empty{display:flex;align-items:center;gap:8px;margin-top:16px;padding:10px 12px;color:var(--ops-muted);background:var(--ops-surface-soft);font-size:11px}.provider-form{margin-top:20px;padding-top:20px;border-top:1px solid var(--ops-border-soft)}.two-column,.three-column{display:grid;grid-template-columns:1fr 1fr;gap:18px}.three-column{grid-template-columns:repeat(3,1fr)}.field-help{display:block;margin-top:6px;color:var(--ops-muted);font-size:10px}.field-help code{color:var(--ops-primary)}:deep(.el-select){width:min(420px,100%)}@media(max-width:680px){.provider-selector,.two-column,.three-column{grid-template-columns:1fr}.settings-header{align-items:flex-end}}
.provider-detail-card{margin-top:20px;padding-top:20px;border-top:1px solid var(--ops-border-soft)}.permission-setting{margin-top:20px;padding-top:20px;border-top:1px solid var(--ops-border-soft)}.permission-setting :deep(.el-select){width:100%;max-width:none}.tooltip-title{display:inline-flex;align-items:center;gap:5px;align-self:flex-start}.help-icon{color:var(--ops-muted);font-size:14px;cursor:help}.help-icon:focus-visible{outline:2px solid color-mix(in srgb,var(--ops-primary) 45%,transparent);outline-offset:2px;border-radius:50%}.settings-actions{display:flex;justify-content:flex-end;margin-top:22px;padding-top:16px;border-top:1px solid var(--ops-border-soft)}
.provider-selector{grid-template-columns:repeat(3,minmax(0,1fr))}.provider-option{text-align:left}.provider-icon.none{color:var(--ops-muted);background:var(--ops-surface-soft);border:1px dashed var(--ops-border);font-size:20px}.editor-shell{overflow:hidden;border:1px solid var(--ops-border);border-radius:7px}.editor-toolbar{display:flex;align-items:center;height:38px;padding:0 10px;background:var(--ops-surface-soft);border-bottom:1px solid var(--ops-border)}.editor-toolbar button{height:38px;padding:0 12px;color:var(--ops-muted);background:transparent;border:0;border-bottom:2px solid transparent;cursor:pointer;font-size:12px}.editor-toolbar button.active{color:var(--ops-primary);border-bottom-color:var(--ops-primary);font-weight:600}.editor-toolbar>span{margin-left:auto;color:var(--ops-muted);font:10px/1 'SF Mono',Monaco,Consolas,monospace}.editor-shell :deep(.code-editor){border:0;border-radius:0}.sample-code{height:340px;box-sizing:border-box;margin:0;padding:16px 18px;overflow:auto;color:#d5deeb;background:#1e1e1e;font:13px/21px 'SF Mono',Monaco,Consolas,monospace;white-space:pre-wrap}.config-note{display:flex;align-items:center;gap:8px;margin-top:12px;color:var(--ops-muted);font-size:11px}.config-note code{color:var(--ops-primary);font-family:'SF Mono',Monaco,Consolas,monospace}
</style>

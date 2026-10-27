<template>
  <main class="login-page">
    <section class="visual-panel" aria-labelledby="platform-title">
      <header class="brand-lockup">
        <img :src="platform.logoData || '/favicon.svg'" class="brand-image" alt="" />
        <div><strong>{{ platform.systemName }}</strong><small>{{ platform.systemSubtitle }}</small></div>
      </header>
      <div class="visual-copy">
        <span class="eyebrow">让运维更从容</span>
        <h1 id="platform-title">洞察全局<span>每一步，清晰可控。</span></h1>
        <p>连接数据、洞察异常，让复杂系统的日常运维更简单。</p>
      </div>
      <div class="landscape" aria-hidden="true"><i></i><i></i><i></i></div>
    </section>

    <section class="login-panel" aria-labelledby="login-title">
      <div class="login-content">
        <div class="login-heading">
          <span v-if="authType !== 'local'" class="auth-badge">{{ authType === 'oidc' ? 'OIDC' : 'LDAP' }}</span>
          <h2 id="login-title">{{ loginTitle }}</h2>
          <p class="login-intro">{{ loginDescription }}</p>
        </div>

        <div v-if="authType === 'oidc'" class="external-login">
          <div class="provider-mark oidc" aria-hidden="true">
            <svg viewBox="0 0 20 20">
              <path class="oidc-orange" d="M9,16.979V1.985L9,.104l3,1.412V18.445Z" />
              <path class="oidc-gray" d="M9,.104V1.985C5.576,2.414,3,4.285,3,6.527c0,2.126,2.316,3.916,5.473,4.462v1.906C3.644,12.311,0,9.682,0,6.527,0,3.258,3.912.554,9,.104ZM19.804,7.73l-.414,4.3-1.543-.869-.217.13a11.928,11.928,0,0,1-5.1,1.6V10.989a9.256,9.256,0,0,0,3.1-1.08L14,8.992Z" />
            </svg>
          </div>
          <el-button type="primary" size="large" :loading="loading" class="external-login-button" @click="startOIDC">
            使用 OIDC 单点登录
            <el-icon class="button-arrow"><Right /></el-icon>
          </el-button>
          <p>将跳转至统一身份认证平台，认证成功后自动返回。</p>
        </div>

        <el-form v-else ref="formRef" :model="form" :rules="rules" label-position="top" size="large" class="login-form" @submit.prevent="handleLogin">
          <el-form-item prop="username">
            <el-input v-model="form.username" :placeholder="authType === 'ldap' ? '请输入 LDAP 用户名' : '请输入用户名'" aria-label="用户名" name="username" prefix-icon="User" autocomplete="username" />
          </el-form-item>
          <el-form-item prop="password">
            <el-input v-model="form.password" type="password" aria-label="密码" name="password" placeholder="请输入密码" prefix-icon="Lock"
              autocomplete="current-password" show-password />
          </el-form-item>
          <el-form-item class="submit-item">
            <el-button type="primary" :loading="loading" class="login-button" native-type="submit">
              {{ authType === 'ldap' ? '使用 LDAP 登录' : '登录' }}
            </el-button>
          </el-form-item>
        </el-form>

        <button v-if="externalAuthType" type="button" class="auth-switch" @click="toggleAuthType">
          {{ authType === 'local' ? externalAuthLabel : '使用系统账号登录' }}
        </button>
      </div>
      <footer class="login-footer">{{ platform.systemName }} · 让运维清晰可控</footer>
    </section>
  </main>
</template>

<script setup>
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { Right } from '@element-plus/icons-vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { usePlatformStore } from '../stores/platform'
import { completeOIDCLogin } from '../api/systemSetting'

const router = useRouter()
const route = useRoute()
const store = useUserStore()
const platform = usePlatformStore()
const formRef = ref()
const loading = ref(false)
const authType = ref('local')

const form = reactive({ username: '', password: '' })

function resetLoginFields() {
  const useDevAccount = import.meta.env.DEV && authType.value === 'local'
  form.username = useDevAccount ? 'admin' : ''
  form.password = useDevAccount ? 'admin123' : ''
}
resetLoginFields()
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

const externalAuthType = computed(() => platform.oidcEnabled ? 'oidc' : platform.ldapEnabled ? 'ldap' : '')
const externalAuthLabel = computed(() => externalAuthType.value === 'oidc' ? '使用 OIDC 单点登录' : '使用 LDAP 账号登录')
const loginTitle = computed(() => authType.value === 'oidc' ? '统一身份认证' : authType.value === 'ldap' ? 'LDAP 登录' : '账号登录')
const loginDescription = computed(() => authType.value === 'oidc'
  ? `通过统一身份认证进入 ${platform.systemName}`
  : authType.value === 'ldap'
    ? `使用企业目录账号进入 ${platform.systemName}`
    : '请登录您的账号')

async function handleLogin() {
  if (loading.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  loading.value = true
  try {
    await store.loginAction({ ...form, auth_type: authType.value })
    router.push('/dashboard')
  } finally {
    loading.value = false
  }
}

function startOIDC() {
  loading.value = true
  window.location.href = '/api/auth/oidc/start'
}

function toggleAuthType() {
  authType.value = authType.value === 'local' ? externalAuthType.value : 'local'
  resetLoginFields()
  nextTick(() => formRef.value?.clearValidate?.())
}

onMounted(async () => {
  if (!platform.loaded) await platform.load()
  authType.value = externalAuthType.value || 'local'
  if (authType.value !== 'local') resetLoginFields()
  if (route.query.oidc !== 'complete') return
  loading.value = true
  try {
    const res = await completeOIDCLogin()
    store.acceptToken(res.data.token)
    await router.replace('/dashboard')
  } finally {
    loading.value = false
  }
})
</script>

<style scoped>
.login-page {
  --ink: var(--ops-text);
  --muted: var(--ops-muted);
  --accent: var(--ops-primary);
  position: relative;
  display: grid;
  grid-template-columns: 1fr 1fr;
  min-height: 100vh;
  min-height: 100dvh;
  overflow: hidden;
  color: var(--ink);
  background: radial-gradient(ellipse at 18% 85%, var(--ops-primary-soft), transparent 55%), var(--ops-bg);
}
.visual-panel { position: relative; display: flex; flex-direction: column; padding: 48px clamp(32px, 5vw, 80px); }
.brand-lockup { position: relative; z-index: 1; display: flex; align-items: center; gap: 12px; }
.brand-image { width: 44px; height: 44px; object-fit: contain; }
.brand-lockup > div { display: flex; flex-direction: column; gap: 4px; }
.brand-lockup strong { font-size: 19px; font-weight: 600; letter-spacing: .04em; }
.brand-lockup small { color: var(--muted); font-size: 11px; letter-spacing: .12em; }
.visual-copy { position: relative; z-index: 1; margin: auto 0; padding: 40px 0 120px; }
.eyebrow { color: #6c83ad; font-size: 13px; letter-spacing: .22em; }
.visual-copy h1 { margin: 22px 0; color: var(--ink); font-size: clamp(38px, 3.5vw, 56px); font-weight: 600; letter-spacing: .02em; line-height: 1.4; }
.visual-copy h1 span { display: block; color: #5c79b0; font-size: .72em; font-weight: 400; margin-top: 8px; }
.visual-copy p { max-width: 390px; color: #8190a7; font-size: 14px; line-height: 1.9; }
.landscape { position: absolute; inset: 0; overflow: hidden; pointer-events: none; }
.landscape i { position: absolute; width: 850px; height: 480px; left: -220px; bottom: -290px; border: 1px solid #ffffffb3; border-radius: 45%; background: linear-gradient(150deg, #ffffff70, #abc7f233); transform: rotate(-28deg); box-shadow: 0 -20px 70px #a6bfe514; }
.landscape i:nth-child(2) { left: -170px; bottom: -340px; transform: rotate(-38deg); }
.landscape i:nth-child(3) { left: -70px; bottom: -390px; transform: rotate(-48deg); }
.login-panel { z-index: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 100px 40px; }
.login-content { width: 100%; max-width: 420px; padding: 40px; border: 1px solid var(--ops-border-soft); border-radius: 16px; background: var(--ops-surface); box-shadow: 0 18px 65px #45628d12, 0 3px 12px #45628d05; }
.login-heading { margin-bottom: 32px; }
.login-content h2 { margin: 0 0 12px; color: var(--ink); font-size: 28px; font-weight: 600; }
.login-intro { margin: 0; color: var(--muted); font-size: 14px; line-height: 1.6; }
.login-form :deep(.el-form-item) { margin-bottom: 24px; }
.login-form :deep(.el-input__wrapper) { min-height: 44px; padding: 0 12px; background: transparent; border-radius: 6px; box-shadow: 0 0 0 1px var(--ops-border) inset; transition: box-shadow .2s; }
.login-form :deep(.el-input__wrapper:hover), .login-form :deep(.el-input__wrapper.is-focus) { box-shadow: 0 0 0 1px var(--accent) inset; }
.login-form :deep(.el-input__inner) { color: var(--ink); font-size: 14px; }
.login-form :deep(.el-input__prefix) { color: #939dac; font-size: 18px; margin-right: 4px; }
.login-form :deep(.submit-item) { margin-top: 32px; margin-bottom: 0; }
.login-button, .external-login-button { width: 100%; height: 44px; font-size: 14px; font-weight: 500; border-radius: 6px; background: var(--accent); border-color: var(--accent); }
.login-button:hover, .external-login-button:hover { background: var(--el-color-primary-dark-2); border-color: var(--el-color-primary-dark-2); }
.auth-badge { display: inline-block; margin-bottom: 16px; padding: 4px 8px; color: var(--accent); background: var(--ops-primary-soft); border-radius: 4px; font-size: 11px; }
.external-login { text-align: center; }
.provider-mark { margin: 0 auto 24px; }
.provider-mark svg { width: 40px; height: 40px; }
.oidc-orange { fill: #f8931e; }.oidc-gray { fill: #929aa5; }
.button-arrow { margin-left: 8px; }
.external-login p { margin: 16px 0 0; color: var(--muted); font-size: 12px; line-height: 1.7; }
.auth-switch { display: block; margin: 24px auto 0; padding: 4px; color: var(--accent); font: inherit; font-size: 13px; border: 0; background: transparent; cursor: pointer; }
.auth-switch:hover { text-decoration: underline; text-underline-offset: 4px; }
.auth-switch:focus-visible { outline: 2px solid var(--accent); outline-offset: 4px; border-radius: 3px; }
.login-footer { position: absolute; bottom: 28px; left: 24px; right: 24px; color: #98a4b8; font-size: 12px; text-align: center; }
:global(html.dark) .landscape { opacity: .14; }
@media (min-width: 1440px) { .login-panel { padding-right: 10vw; } }
@media (max-width: 900px) { .visual-panel { padding: 32px; }.visual-copy h1 { font-size: 36px; }.login-panel { padding-inline: 24px; }.login-content { padding: 32px; } }
@media (max-width: 680px) {
  .login-page { grid-template-columns: 1fr; grid-template-rows: auto 1fr; }
  .visual-panel { padding: 28px 24px 0; }.visual-copy, .landscape { display: none; }
  .brand-image { width: 36px; height: 36px; }.brand-lockup strong { font-size: 17px; }
  .login-panel { padding: 44px 24px 88px; }.login-content { padding: 32px 28px; }
  .login-content h2 { font-size: 26px; }.login-footer { bottom: 24px; font-size: 11px; }
}
</style>

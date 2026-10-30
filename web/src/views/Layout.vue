<template>
  <el-container class="layout" :style="{ '--sidebar-width': isCollapse ? '72px' : '220px' }">
    <el-aside :width="isCollapse ? '72px' : '220px'" class="sidebar">
      <router-link class="logo" :class="{ 'is-collapsed': isCollapse }" to="/dashboard" :aria-label="`${platform.systemName}，返回首页`" :title="platform.systemName">
        <img v-if="platform.logoData" class="logo-image" :src="platform.logoData" alt="" />
        <span v-else class="logo-mark" aria-hidden="true">
          <img src="/favicon.svg" alt="" />
        </span>
        <span v-if="!isCollapse" class="logo-name">{{ platform.systemName }}</span>
      </router-link>
      <div class="sidebar-menu">
        <el-menu :default-active="activeMenu" router :collapse="isCollapse" :collapse-transition="false">
          <template v-for="menu in menus" :key="menu.id">
            <el-sub-menu v-if="menu.children && menu.children.length" :index="String(menu.id)">
              <template #title>
                <el-icon><component :is="menu.icon" /></el-icon>
                <span>{{ menu.name }}</span>
              </template>
              <template v-for="child in menu.children" :key="child.id">
                <el-menu-item v-if="child.type === 2" :index="child.path">
                  <span>{{ child.name }}</span>
                </el-menu-item>
              </template>
            </el-sub-menu>
            <el-menu-item v-else :index="menu.path">
              <el-icon><component :is="menu.icon" /></el-icon>
              <span>{{ menu.name }}</span>
            </el-menu-item>
          </template>
        </el-menu>
      </div>
      <div class="sidebar-footer">
        <button class="collapse-btn" type="button" @click="isCollapse = !isCollapse" aria-label="切换导航">
          <ArrowLeftBold v-if="!isCollapse" />
          <ArrowRightBold v-else />
        </button>
      </div>
    </el-aside>

    <div ref="workspaceBody" class="workspace-shell" :class="{ 'is-resizing': resizing }">
      <el-container class="layout-workspace">
        <el-header class="header">
          <div class="header-left">
            <el-breadcrumb separator="/">
              <el-breadcrumb-item :to="{ path: '/dashboard' }">首页</el-breadcrumb-item>
              <el-breadcrumb-item v-if="breadcrumbParent" :to="{ path: breadcrumbParent.path }">{{ breadcrumbParent.title }}</el-breadcrumb-item>
              <el-breadcrumb-item v-if="currentTitle">{{ currentTitle }}</el-breadcrumb-item>
            </el-breadcrumb>
          </div>
          <div class="header-right">
            <a class="theme-trigger" href="https://github.com/darianJmy/awesome-alert" target="_blank" rel="noopener noreferrer" aria-label="GitHub 项目仓库（新标签页打开）" title="GitHub 项目仓库">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M12 .75a11.25 11.25 0 0 0-3.558 21.923c.563.104.768-.244.768-.542 0-.267-.01-.974-.015-1.912-3.13.68-3.79-1.508-3.79-1.508-.512-1.3-1.25-1.646-1.25-1.646-1.023-.7.078-.686.078-.686 1.13.08 1.725 1.16 1.725 1.16 1.005 1.722 2.638 1.225 3.28.937.102-.728.393-1.225.715-1.507-2.498-.284-5.124-1.249-5.124-5.563 0-1.23.44-2.234 1.16-3.022-.116-.285-.503-1.43.11-2.98 0 0 .945-.303 3.094 1.154A10.79 10.79 0 0 1 12 6.18c.956.004 1.918.129 2.817.378 2.148-1.457 3.091-1.154 3.091-1.154.615 1.55.228 2.695.112 2.98.722.788 1.158 1.792 1.158 3.022 0 4.325-2.63 5.276-5.136 5.555.404.35.765 1.042.765 2.1 0 1.516-.014 2.74-.014 3.112 0 .3.203.651.774.541A11.252 11.252 0 0 0 12 .75Z" />
              </svg>
            </a>
            <button ref="aiTrigger" type="button" class="theme-trigger ai-header-trigger" :class="{ 'is-active': aiVisible }" :aria-expanded="aiVisible" aria-controls="guanlan-ai-panel" aria-label="AI 助手" title="AI 助手" @click="aiVisible = !aiVisible">
              <svg width="17" height="17" viewBox="0 0 24 24" aria-hidden="true">
                <g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                  <path d="M12 8V4H8" />
                  <rect width="16" height="12" x="4" y="8" rx="2" />
                  <path d="M2 14h2m16 0h2m-7-1v2m-6-2v2" />
                </g>
              </svg>
            </button>
            <el-dropdown trigger="click" placement="bottom-end" @command="handleThemeCommand">
              <button type="button" class="theme-trigger" aria-label="选择主题" title="选择主题">
                <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" aria-hidden="true" focusable="false">
                  <g fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                    <path d="M12 22a1 1 0 0 1 0-20a10 9 0 0 1 10 9a5 5 0 0 1-5 5h-2.25a1.75 1.75 0 0 0-1.4 2.8l.3.4a1.75 1.75 0 0 1-1.4 2.8z" />
                    <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" />
                    <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" />
                    <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" />
                    <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" />
                  </g>
                </svg>
              </button>
              <template #dropdown>
                <el-dropdown-menu class="theme-menu">
                  <el-dropdown-item v-for="theme in themes" :key="theme.id" :command="theme.id" :class="{ 'is-selected': activeTheme === theme.id }" :aria-label="`${theme.label}${activeTheme === theme.id ? '，已选择' : ''}`">
                    <el-icon><component :is="theme.dark ? 'Moon' : 'Sunny'" /></el-icon>
                    <span>{{ theme.label }}</span>
                    <el-icon v-if="activeTheme === theme.id" class="theme-check"><Check /></el-icon>
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
            <el-dropdown @command="handleCommand">
              <span class="user-info">
                <span class="user-avatar">{{ userInitial }}</span>
                <span class="user-name">{{ store.userInfo?.nickname || store.userInfo?.username }}</span>
                <el-icon><ArrowDown /></el-icon>
              </span>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="profile">个人信息</el-dropdown-item>
                  <el-dropdown-item command="password">修改密码</el-dropdown-item>
                  <el-dropdown-item divided command="logout">退出登录</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </el-header>

        <div class="workspace-body">
          <el-main>
            <router-view />
          </el-main>
        </div>
      </el-container>
      <aside v-show="aiVisible" id="guanlan-ai-panel" class="ai-aside-panel" :style="{ width: `${panelWidth}px` }" aria-label="AI 助手侧栏" @keydown.esc="closeAI">
        <div class="ai-panel-resizer" role="separator" tabindex="0" aria-label="调整 AI 助手宽度" aria-orientation="vertical" :aria-valuenow="Math.round(panelWidth)" :aria-valuemin="320" :aria-valuemax="Math.round(maxPanelWidth)" @pointerdown="startResize" @pointermove="resizePanel" @pointerup="stopResize" @pointercancel="stopResize" @lostpointercapture="resizing = false" @keydown.left.prevent="changePanelWidth(panelWidth + 24)" @keydown.right.prevent="changePanelWidth(panelWidth - 24)" @keydown.home.prevent="changePanelWidth(320)" @keydown.end.prevent="changePanelWidth(maxPanelWidth)" />
        <AIChatAssistant :visible="aiVisible" @close="closeAI" />
      </aside>
    </div>
  </el-container>


  <!-- 修改密码弹窗 -->
  <el-dialog v-model="passwordVisible" title="修改密码" width="400px" :close-on-click-modal="false">
    <el-form ref="pwdFormRef" :model="pwdForm" :rules="pwdRules" label-width="80px">
      <el-form-item label="原密码" prop="old_password">
        <el-input v-model="pwdForm.old_password" type="password" show-password />
      </el-form-item>
      <el-form-item label="新密码" prop="new_password">
        <el-input v-model="pwdForm.new_password" type="password" show-password />
      </el-form-item>
      <el-form-item label="确认密码" prop="confirm_password">
        <el-input v-model="pwdForm.confirm_password" type="password" show-password />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="passwordVisible = false">取消</el-button>
      <el-button type="primary" @click="handleChangePassword">确认</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { themes } from '../theme'
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'
import { usePlatformStore } from '../stores/platform'
import { changePassword } from '../api/auth'
import { ElMessage } from 'element-plus'
import AIChatAssistant from '../components/AIChatAssistant.vue'
import { pageBreadcrumb } from '../composables/usePageBreadcrumb'

const route = useRoute()
const router = useRouter()
const store = useUserStore()
const platform = usePlatformStore()

const aiVisible = ref(false)
const aiTrigger = ref()
const workspaceBody = ref()
const panelWidth = ref(460)
const maxPanelWidth = ref(840)
const resizing = ref(false)
let resizeStartX = 0
let resizeStartWidth = 0
let bodyObserver

function closeAI() {
  aiVisible.value = false
  resizing.value = false
  aiTrigger.value?.focus()
}

function changePanelWidth(width) {
  panelWidth.value = Math.min(maxPanelWidth.value, Math.max(320, width))
  try { localStorage.setItem('guanlan_ai_panel_width', String(panelWidth.value)) } catch {}
}

function startResize(event) {
  if (event.button !== 0) return
  resizing.value = true
  resizeStartX = event.clientX
  resizeStartWidth = panelWidth.value
  event.currentTarget.setPointerCapture(event.pointerId)
  event.preventDefault()
}

function resizePanel(event) {
  if (resizing.value) changePanelWidth(resizeStartWidth + resizeStartX - event.clientX)
}

function stopResize(event) {
  resizing.value = false
  if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
}

onMounted(() => {
  try {
    const saved = Number(localStorage.getItem('guanlan_ai_panel_width'))
    if (Number.isFinite(saved) && saved >= 320) panelWidth.value = saved
  } catch {}
  bodyObserver = new ResizeObserver(([entry]) => {
    if (window.innerWidth <= 960) isCollapse.value = true
    maxPanelWidth.value = Math.max(320, Math.min(840, entry.contentRect.width - 360))
    panelWidth.value = Math.min(maxPanelWidth.value, Math.max(320, panelWidth.value))
  })
  bodyObserver.observe(workspaceBody.value)
})
onBeforeUnmount(() => bodyObserver?.disconnect())

const isCollapse = ref(window.innerWidth <= 960)
const menus = computed(() => store.menus)
const breadcrumbParent = computed(() => route.meta.breadcrumbParent)
const currentTitle = computed(() => (pageBreadcrumb.value?.path === route.path && pageBreadcrumb.value.title) || route.meta.title)
const activeMenu = computed(() => route.meta.activeMenu || route.path)
const activeTheme = computed(() => platform.themePreference || platform.defaultTheme)
const userInitial = computed(() => {
  const name = store.userInfo?.nickname || store.userInfo?.username || 'U'
  return String(name).slice(0, 1).toUpperCase()
})

const passwordVisible = ref(false)
const pwdFormRef = ref()
const pwdForm = ref({ old_password: '', new_password: '', confirm_password: '' })

const validateConfirm = (rule, value, callback) => {
  if (value !== pwdForm.value.new_password) {
    callback(new Error('两次密码不一致'))
  } else {
    callback()
  }
}

const pwdRules = {
  old_password: [{ required: true, message: '请输入原密码', trigger: 'blur' }],
  new_password: [{ required: true, min: 6, message: '密码至少6位', trigger: 'blur' }],
  confirm_password: [{ required: true, validator: validateConfirm, trigger: 'blur' }],
}

function handleCommand(cmd) {
  if (cmd === 'logout') {
    store.logout()
    router.push('/login')
  } else if (cmd === 'password') {
    pwdForm.value = { old_password: '', new_password: '', confirm_password: '' }
    passwordVisible.value = true
  }
}

function handleThemeCommand(theme) {
  platform.setThemePreference(theme)
}

async function handleChangePassword() {
  const valid = await pwdFormRef.value.validate().catch(() => false)
  if (!valid) return
  try {
    await changePassword({
      old_password: pwdForm.value.old_password,
      new_password: pwdForm.value.new_password,
    })
    ElMessage.success('密码修改成功')
    passwordVisible.value = false
  } catch {}
}
</script>

<style scoped>
.workspace-shell{display:flex;flex:1;min-width:0;min-height:0;overflow:hidden}
.layout-workspace{min-width:0;min-height:0;flex-direction:column}
.workspace-body{position:relative;display:flex;flex:1;min-width:0;min-height:0;overflow:hidden}
.ai-aside-panel{position:relative;flex:none;min-width:0;min-height:0;max-width:100%;border-left:1px solid var(--ops-border);background:var(--ops-surface)}
.ai-panel-resizer{position:absolute;top:0;bottom:0;left:-4px;width:8px;z-index:2;cursor:col-resize;touch-action:none}
.ai-panel-resizer:hover,.ai-panel-resizer:focus-visible,.is-resizing .ai-panel-resizer{background:color-mix(in srgb,var(--ops-primary) 32%,transparent);outline:none}
.is-resizing{user-select:none;cursor:col-resize}
.header-right{flex-shrink:0}
@media(max-width:600px){.header{padding:0 12px!important}.header-left{overflow:hidden}.header-right{gap:6px!important}.user-name{display:none}}
.ai-header-trigger.is-active{color:var(--ops-primary);background:var(--ops-primary-soft)}
@media(max-width:960px){.ai-aside-panel{position:fixed;z-index:1900;top:64px;right:0;bottom:0;width:min(460px,100vw)!important;box-shadow:-8px 0 24px rgba(0,0,0,.12)}.ai-panel-resizer{display:none}}

.layout {
  --layout-header-height: 64px;
  height: 100vh;
  background: var(--ops-bg);
}

.sidebar {
  flex-shrink: 0;
  transition: width 180ms ease;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  overflow-x: hidden;
  background: var(--ops-surface);
  border-right: 1px solid var(--ops-border);
  box-shadow: 8px 0 28px rgba(16, 24, 40, 0.04);
}

.logo {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-width: 0;
  box-sizing: border-box;
  flex: 0 0 var(--layout-header-height);
  height: var(--layout-header-height);
  padding: 0 18px;
  color: inherit;
  text-decoration: none;
  background: var(--ops-surface);
  border-bottom: 1px solid var(--ops-border);
  cursor: pointer;
  transition: background-color 0.18s ease;
}

.logo:hover {
  background: var(--ops-primary-soft);
}

.logo:focus-visible {
  outline: 2px solid var(--ops-primary);
  outline-offset: -3px;
}

.logo-mark {
  display: grid;
  place-items: center;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
}
.logo-mark img { display: block; width: 38px; height: 38px; }
.logo-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--ops-text);
  font-size: 16px;
  font-weight: 750;
  letter-spacing: 0.02em;
}
.logo-image { flex: 0 0 38px; width: 38px; height: 38px; object-fit: contain; }
.logo.is-collapsed { padding: 0 10px; }

.sidebar-menu {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

.sidebar :deep(.el-menu) {
  box-sizing: border-box;
  width: 100%;
  transition: none;
  border-right: 0;
  padding: 10px;
}

.sidebar :deep(.el-menu--collapse) {
  box-sizing: border-box;
  width: 100%;
}

.sidebar :deep(.el-menu--collapse > .el-menu-item),
.sidebar :deep(.el-menu--collapse > .el-sub-menu > .el-sub-menu__title),
.sidebar :deep(.el-menu--collapse > .el-menu-item > .el-menu-tooltip__trigger) {
  display: flex;
  align-items: center;
  justify-content: center;
  padding-left: 0;
  padding-right: 0;
}

.sidebar :deep(.el-menu--collapse > .el-menu-item .el-icon),
.sidebar :deep(.el-menu--collapse > .el-sub-menu > .el-sub-menu__title > .el-icon) {
  margin: 0;
  flex-shrink: 0;
}

.sidebar :deep(.el-menu-item),
.sidebar :deep(.el-sub-menu__title) {
  height: 42px;
  margin: 4px 0;
  border-radius: 8px;
  color: var(--ops-muted);
  font-weight: 650;
}

@media (prefers-reduced-motion: reduce) {
  .sidebar { transition: none; }
}

.sidebar :deep(.el-menu-item:hover),
.sidebar :deep(.el-sub-menu__title:hover) {
  color: var(--ops-primary);
  background: var(--ops-primary-soft);
}

.sidebar :deep(.el-menu-item.is-active) {
  color: var(--ops-primary);
  background: var(--ops-primary-soft);
}

.sidebar-footer {
  flex: 0 0 auto;
  display: flex;
  justify-content: center;
  padding: 8px 0 10px;
  border-top: 1px solid var(--ops-border-soft);
}

.sidebar-footer .collapse-btn {
  width: 30px;
  height: 30px;
}

.header {
  box-sizing: border-box;
  flex: 0 0 var(--layout-header-height);
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--layout-header-height);
  padding: 0 24px;
  background: color-mix(in srgb, var(--ops-surface) 86%, transparent);
  border-bottom: 1px solid var(--ops-border);
  backdrop-filter: blur(12px);
}

.header-left {
  display: flex;
  align-items: center;
  gap: 14px;
  min-width: 0;
}

.collapse-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 30px;
  height: 30px;
  padding: 0;
  color: var(--ops-subtle);
  cursor: pointer;
  background: transparent;
  border: 0;
  border-radius: 8px;
  transition: color 0.16s ease, background 0.16s ease;
}

.collapse-btn svg {
  width: 13px;
  height: 13px;
}

.collapse-btn:hover {
  color: var(--ops-primary);
  background: var(--ops-primary-soft);
}

.collapse-btn:active {
  transform: translateY(1px);
}

.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.theme-trigger {
  display: inline-grid;
  width: 32px;
  height: 32px;
  place-items: center;
  padding: 0;
  color: var(--ops-muted);
  background: transparent;
  border: 0;
  border-radius: 8px;
  cursor: pointer;
  font-size: 17px;
  transition: color .16s ease, background-color .16s ease;
}

.theme-trigger:hover,
.theme-trigger:focus-visible {
  color: var(--ops-primary);
  background: var(--ops-primary-soft);
}

.theme-trigger:focus-visible {
  outline: 2px solid var(--ops-primary);
  outline-offset: 2px;
}

.user-info {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ops-text);
  cursor: pointer;
  font-weight: 650;
}

.user-avatar {
  display: inline-grid;
  place-items: center;
  width: 30px;
  height: 30px;
  color: var(--ops-surface);
  background: var(--ops-text);
  border-radius: 999px;
  font-size: 12px;
  font-weight: 800;
}

.el-main {
  min-width: 0;
  padding: 22px 24px;
  background: var(--ops-bg);
}

</style>

<style>
.theme-menu .el-dropdown-menu__item {
  display: grid;
  min-width: 148px;
  padding: 9px 14px;
  grid-template-columns: 20px 1fr 18px;
  gap: 8px;
}

.theme-menu .is-selected { background: var(--ops-primary-soft); color: var(--ops-primary); }
.theme-menu .theme-check {
  color: var(--ops-primary);
}
</style>

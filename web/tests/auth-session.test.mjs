import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import vm from 'node:vm'
import { createPinia, defineStore } from 'pinia'
import { createRouter, createMemoryHistory } from 'vue-router'

// Run the real store and router with memory history; only HTTP, storage and
// lazy-loaded page components are replaced so this regression needs no browser.
function session() {
  let token = ''
  let calls = 0
  let failProfile = false
  const context = vm.createContext({
    defineStore,
    createRouter,
    createWebHistory: createMemoryHistory,
    document: { title: '' },
    getToken: () => token,
    setToken: value => { token = value },
    removeToken: () => { token = '' },
    login: async ({ username }) => ({ data: { token: username } }),
    getUserInfo: async () => {
      calls++
      if (failProfile) throw new Error('Profile unavailable')
      const admin = token === 'admin'
      return { data: {
        user: { username: token },
        permissions: admin ? ['dashboard:list', 'user:list'] : ['dashboard:list'],
        menus: admin ? [{ type: 2, path: '/system/users', name: 'Users', component: 'system/UserManage' }] : [],
      } }
    },
  })
  const stripImports = source => source.replace(/^import .* from .*\n/gm, '')
  const storeSource = stripImports(readFileSync(new URL('../src/stores/user.js', import.meta.url), 'utf8'))
    .replace('export const useUserStore', 'const useUserStore')
  const useStore = vm.runInContext(storeSource + '\nuseUserStore', context)
  const store = useStore(createPinia())
  context.sessionStore = () => store
  const routerSource = stripImports(readFileSync(new URL('../src/router/index.js', import.meta.url), 'utf8'))
    .replaceAll('useUserStore()', 'sessionStore()')
    .replace(/\(\) => import\('[^']+'\)/g, '() => Promise.resolve({ render() {} })')
    .replace('export default router', 'router')
  const router = vm.runInContext(routerSource, context)
  return { store, router, calls: () => calls, failProfile: () => { failProfile = true } }
}

test('logout and login again reload dashboard permissions and menus', async () => {
  const { store, router, calls } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/dashboard')
  assert.equal(store.hasPermission('dashboard:list'), true)
  assert.equal(store.menus.length, 1)
  assert.equal(calls(), 1)
  await router.push('/system/users')
  assert.equal(calls(), 1, 'ordinary navigation should reuse the loaded profile')

  store.logout()
  await router.push('/login')
  await store.loginAction({ username: 'admin' })
  await router.push('/dashboard')
  assert.equal(calls(), 2, 'a new login must load the profile again')
  assert.equal(store.hasPermission('dashboard:list'), true)
  assert.equal(store.menus.length, 1)
  assert.equal(router.hasRoute('Users'), true)
})

test('logout removes old account routes and a different account gets its own permissions', async () => {
  const { store, router } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/dashboard')
  assert.equal(router.hasRoute('Users'), true)
  store.logout()
  await router.push('/login')
  assert.equal(router.hasRoute('Users'), false)
  await store.loginAction({ username: 'viewer' })
  await router.push('/dashboard')
  assert.equal(store.userInfo.username, 'viewer')
  assert.equal(store.hasPermission('dashboard:list'), true)
  assert.equal(store.hasPermission('user:list'), false)
  assert.equal(store.menus.length, 0)
  assert.equal(router.hasRoute('Users'), false)
})

test('accepting an SSO token invalidates the previous profile', async () => {
  const { store, router, calls } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/system/users')
  store.acceptToken('viewer')
  await router.push('/dashboard')
  assert.equal(calls(), 2)
  assert.equal(store.userInfo.username, 'viewer')
  assert.equal(store.hasPermission('user:list'), false)
  assert.equal(router.hasRoute('Users'), false)
})

test('failed profile loading returns to login with no stale authorization', async () => {
  const { store, router, failProfile } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/dashboard')
  store.logout()
  await router.push('/login')
  failProfile()
  await store.loginAction({ username: 'admin' })
  await router.push('/dashboard')
  assert.equal(router.currentRoute.value.path, '/login')
  assert.equal(store.isLoggedIn, false)
  assert.equal(store.userInfo, null)
  assert.equal(store.permissions.length, 0)
  assert.equal(router.hasRoute('Users'), false)
})

test('removed pages show the not-found page after loading the profile', async () => {
  const { store, router } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/settings/mcp')
  assert.equal(router.currentRoute.value.name, 'NotFound')
  assert.equal(router.currentRoute.value.path, '/settings/mcp')
  assert.equal(store.hasPermission('dashboard:list'), true)
})

test('unknown URLs show 404 and can return to the dashboard', async () => {
  const { store, router, calls } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/alert-rulessss3?source=bookmark#details')
  assert.equal(router.currentRoute.value.name, 'NotFound')
  assert.equal(router.currentRoute.value.fullPath, '/alert-rulessss3?source=bookmark#details')
  assert.equal(calls(), 1)
  assert.equal(router.currentRoute.value.matched[0].name, 'Layout')
  await router.push('/dashboard')
  assert.equal(router.currentRoute.value.name, 'Dashboard')
})

test('direct entry to a dynamic page resolves after profile loading', async () => {
  const { store, router } = session()
  await store.loginAction({ username: 'admin' })
  await router.push('/system/users')
  assert.equal(router.currentRoute.value.name, 'Users')
  assert.notEqual(router.currentRoute.value.name, 'NotFound')
})

test('unknown URLs still require authentication', async () => {
  const { router } = session()
  await router.push('/alert-rulessss3')
  assert.equal(router.currentRoute.value.name, 'Login')
})

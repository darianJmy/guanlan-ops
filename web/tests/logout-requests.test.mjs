import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'
import axios from 'axios'
import { compile, createRenderer, defineComponent, h, nextTick, onMounted, reactive } from 'vue'

function client() {
  let token = 'first-session'
  const messages = []
  const location = { href: '' }
  const source = readFileSync(new URL('../src/api/request.js', import.meta.url), 'utf8')
    .replace(/^import .*\n/gm, '').replace('export default request', 'request')
  const request = vm.runInNewContext(source, {
    axios, getToken: () => token, removeToken: () => { token = '' },
    ElMessage: { error: message => messages.push(message) }, window: { location }, Blob,
  })
  return { request, messages, location, setToken: value => { token = value } }
}

for (const kind of ['http-error', 'business-error', 'success']) {
  test(`logout discards a delayed ${kind} without showing errors`, async () => {
    const state = client()
    let finish
    const pending = state.request.get('/system/auth-settings', { adapter: config => new Promise((resolve, reject) => {
      finish = () => {
        const response = { config, status: kind === 'http-error' ? 401 : 200, data: { code: kind === 'success' ? 0 : 401, message: '未提供认证令牌' } }
        if (kind === 'http-error') reject(new axios.AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, null, response))
        else resolve(response)
      }
    }) })
    const assertion = assert.rejects(pending, error => axios.isCancel(error))
    await new Promise(resolve => setImmediate(resolve))
    state.setToken('')
    finish()
    await assertion
    assert.deepEqual(state.messages, [])
    assert.equal(state.location.href, '')
  })
}

test('login failures still show the error to the user', async () => {
  const state = client()
  state.setToken('')
  await assert.rejects(state.request.post('/login', {}, { adapter: async config => ({ config, data: { code: 401, message: '密码错误' } }) }))
  assert.deepEqual(state.messages, ['密码错误'])
})

for (const blob of [false, true]) {
  test(`HTTP 401 clears the current session (blob=${blob})`, async () => {
    const state = client()
    await assert.rejects(state.request.get('/protected', { adapter: async config => {
      const data = { code: 401, message: '认证令牌无效或已过期' }
      const response = { config, status: 401, data: blob ? new Blob([JSON.stringify(data)]) : data }
      throw new axios.AxiosError('Unauthorized', 'ERR_BAD_REQUEST', config, null, response)
    } }))
    assert.equal(state.location.href, '/login')
    assert.deepEqual(state.messages, ['认证令牌无效或已过期'])
    await state.request.get('/public', { adapter: async config => {
      assert.equal(config.headers.Authorization, undefined)
      return { config, data: { code: 0 } }
    } })
  })
}

test('clearing menus during logout does not remount the settings page', async () => {
  const source = readFileSync(new URL('../src/views/Layout.vue', import.meta.url), 'utf8')
  const template = source.match(/<el-main\b[^>]*>[\s\S]*?<\/el-main>/)[0]
  let mounts = 0
  const store = reactive({ userInfo: { id: 1 } })
  const state = reactive({ isSettingsPage: true })
  const page = defineComponent({ setup() { onMounted(() => { mounts++ }); return () => h('div') } })
  const shell = defineComponent({ setup(_, { slots }) { return () => h('section', slots.default?.()) } })
  const renderer = createRenderer({
    createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
    insert() {}, remove() {}, setText() {}, setElementText() {}, patchProp() {},
    parentNode: () => null, nextSibling: () => null,
  })
  const app = renderer.createApp({
    components: { ElMain: shell, SettingsWorkspace: shell, RouterView: page },
    setup: () => ({ store, route: { path: '/settings/authentication' }, settingsItems: [], get isSettingsPage() { return state.isSettingsPage } }),
    render: compile(template),
  })
  app.mount({})
  assert.equal(mounts, 1)
  store.userInfo = null
  state.isSettingsPage = false
  await nextTick()
  assert.equal(mounts, 1, 'logout must not mount another page that sends unauthenticated requests')
  app.unmount()
})

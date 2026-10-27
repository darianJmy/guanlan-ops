import { defineStore } from 'pinia'
import { login, getUserInfo } from '../api/auth'
import { setToken, removeToken, getToken } from '../utils/auth'

export const useUserStore = defineStore('user', {
  state: () => ({
    token: getToken() || '',
    userInfo: null,
    permissions: [],
    menus: [],
  }),
  getters: {
    isLoggedIn: (state) => !!state.token,
  },
  actions: {
    acceptToken(token) {
      this.userInfo = null
      this.permissions = []
      this.menus = []
      this.token = token
      setToken(token)
    },

    async loginAction(credentials) {
      const res = await login(credentials)
      this.acceptToken(res.data.token)
      return res
    },

    async fetchUserInfo() {
      const res = await getUserInfo()
      this.userInfo = res.data.user
      this.permissions = res.data.permissions
      const normalizeMenuNames = menus => (menus || []).map(menu => ({
        ...menu,
        name: menu.path === '/settings/ai-models' && menu.name === 'AI 模型' ? '模型' : menu.name,
        ...(menu.children ? { children: normalizeMenuNames(menu.children) } : {}),
      }))
      this.menus = normalizeMenuNames(res.data.menus)
      return res.data
    },

    hasPermission(code) {
      return this.permissions.includes(code)
    },

    logout() {
      this.token = ''
      this.userInfo = null
      this.permissions = []
      this.menus = []
      removeToken()
    },
  },
})

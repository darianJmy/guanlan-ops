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
      this.menus = res.data.menus
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

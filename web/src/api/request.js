import axios from 'axios'
import { getToken, removeToken } from '../utils/auth'
import { ElMessage } from 'element-plus'

const request = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

request.interceptors.request.use((config) => {
  const token = getToken()
  config.sessionToken = token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

request.interceptors.response.use(
  (response) => {
    if (isPreviousSession(response.config)) {
      return Promise.reject(new axios.CanceledError('登录会话已切换', response.config))
    }
    if (response.config.responseType === 'blob') {
      return response
    }
    const { data } = response
    if (data.code !== 0) {
      if (!response.config.silentError) ElMessage.error(data.message || '请求失败')
      if (data.code === 401) {
        removeToken()
        window.location.href = '/login'
      }
      return Promise.reject(new Error(data.message))
    }
    return data
  },
  (error) => {
    if (axios.isCancel(error)) return Promise.reject(error)
    if (isPreviousSession(error.config)) {
      return Promise.reject(new axios.CanceledError('登录会话已切换', error.config))
    }
    if (error.response?.status === 401) {
      removeToken()
      window.location.href = '/login'
    }
    if (error.response?.data instanceof Blob) {
      return error.response.data.text().then((text) => {
        let message = statusMessage(error.response.status) || '下载失败'
        try {
          message = JSON.parse(text).message || message
        } catch {
          message = text?.trim() && !text.includes('404 page not found') ? text.trim() : message
        }
        ElMessage.error(message)
        return Promise.reject(error)
      })
    }
    const message = error.response?.data?.message || statusMessage(error.response?.status) || error.message || '网络错误'
    if (!error.config?.silentError) ElMessage.error(message)
    return Promise.reject(error)
  }
)

function isPreviousSession(config) {
  return !!config?.sessionToken && config.sessionToken !== getToken()
}

function statusMessage(status) {
  const map = {
    400: '请求参数错误',
    401: '登录已过期，请重新登录',
    403: '没有操作权限',
    404: '接口不存在或后端服务未更新',
    500: '服务异常，请联系管理员',
  }
  return map[status] || ''
}

export default request

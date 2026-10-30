import request from './request'

export const getAutomationTasks = () => request({ url: '/automation-tasks', method: 'get' })
export const createAutomationTask = data => request({ url: '/automation-tasks', method: 'post', data })
export const updateAutomationTask = (id, data) => request({ url: `/automation-tasks/${id}`, method: 'put', data })
export const deleteAutomationTask = id => request({ url: `/automation-tasks/${id}`, method: 'delete' })
export const runAutomationTask = (id, data = {}) => request({ url: `/automation-tasks/${id}/run`, method: 'post', data })
export const getAutomationExecutions = params => request({ url: '/automation-executions', method: 'get', params })

export const getAutomationTaskOptions = () => request({ url: '/automation-tasks/options', method: 'get' })

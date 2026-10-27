import request from './request'

export function getDashboards(params) {
  return request({ url: '/dashboards', method: 'get', params })
}

export function createDashboard(data) {
  return request({ url: '/dashboards', method: 'post', data })
}

export function updateDashboard(id, data) {
  return request({ url: `/dashboards/${id}`, method: 'put', data })
}

export function deleteDashboard(id) {
  return request({ url: `/dashboards/${id}`, method: 'delete' })
}

export function getSLSDashboardProjects() {
  return request({ url: '/sls-dashboards/projects', method: 'get' })
}

export function createSLSDashboardProject(data) {
  return request({ url: '/sls-dashboards/projects', method: 'post', data })
}

export function updateSLSDashboardProject(id, data) {
  return request({ url: `/sls-dashboards/projects/${id}`, method: 'put', data })
}

export function reorderSLSDashboardProjects(ids) {
  return request({ url: '/sls-dashboards/projects/order', method: 'put', data: { ids } })
}

export function deleteSLSDashboardProject(id) {
  return request({ url: `/sls-dashboards/projects/${id}`, method: 'delete' })
}

export function testSLSDashboardProject(data) {
  return request({ url: '/sls-dashboards/projects/test', method: 'post', data })
}

export function getSLSDashboardLogstores(projectId) {
  return request({ url: `/sls-dashboards/${projectId}/logstores`, method: 'get' })
}

export function createSLSDashboardSession(projectId, logstore) {
  return request({
    url: `/sls-dashboards/${projectId}/logstores/${encodeURIComponent(logstore)}/session`,
    method: 'post',
  })
}

export function getCloudMonitorWorkspaces(params) {
  return request({ url: '/cloud-monitor-workspaces', method: 'get', params })
}

export function createCloudMonitorWorkspace(data) {
  return request({ url: '/cloud-monitor-workspaces', method: 'post', data })
}

export function updateCloudMonitorWorkspace(id, data) {
  return request({ url: `/cloud-monitor-workspaces/${id}`, method: 'put', data })
}

export function deleteCloudMonitorWorkspace(id) {
  return request({ url: `/cloud-monitor-workspaces/${id}`, method: 'delete' })
}

export function testCloudMonitorWorkspace(data) {
  return request({ url: '/cloud-monitor-workspaces/test', method: 'post', data })
}

export function createCloudMonitorSession(id) {
  return request({ url: `/cloud-monitor-workspaces/${id}/session`, method: 'post' })
}

export function getDashboardOverview(range = 'today') {
  return request({ url: '/dashboard/overview', method: 'get', params: { range } })
}

export function getDashboardCards() {
  return request({ url: '/dashboard-cards', method: 'get' })
}

export function reorderDashboardCards(items) {
  return request({ url: '/dashboard-cards/order', method: 'put', data: { items } })
}

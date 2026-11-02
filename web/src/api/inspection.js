import request from './request'

export function getInspectionItems(params) {
  return request({ url: '/inspection-items', method: 'get', params })
}

export function exportInspectionItems(params) {
  return request({ url: '/inspection-items/export', method: 'get', params, responseType: 'blob', timeout: 30000 })
}

export function importInspectionItems(content, overwrite, dryRun = false, groupId) {
  return request({ url: '/inspection-items/import', method: 'post', data: { content, overwrite, dry_run: dryRun, group_id: groupId }, timeout: 30000 })
}

export function createInspectionItem(data) {
  return request({ url: '/inspection-items', method: 'post', data })
}

export function updateInspectionItem(id, data) {
  return request({ url: `/inspection-items/${id}`, method: 'put', data })
}

export function batchUpdateInspectionItemStatus(ids, status) {
  return request({ url: '/inspection-items/batch/status', method: 'put', data: { ids, status } })
}

export function batchDeleteInspectionItems(ids) {
  return request({ url: '/inspection-items/batch', method: 'delete', data: { ids } })
}

export function deleteInspectionItem(id) {
  return request({ url: `/inspection-items/${id}`, method: 'delete' })
}

export function runInspectionItem(id, timeoutMs = 300000, nodeCount = 1) {
  // The backend executes up to eight nodes concurrently.
  const timeout = Math.ceil(Math.max(1, nodeCount) / 8) * (timeoutMs + 15000)
  return request({ url: `/inspection-items/${id}/run`, method: 'post', timeout })
}

export function runAllInspections() {
  return request({ url: '/inspection-items/run-all', method: 'post', timeout: 2200000 })
}

export function getInspectionRecords(params) {
  return request({ url: '/inspection-records', method: 'get', params })
}

export function getInspectionRuleGroups(params) {
  return request({ url: '/inspection-rule-groups', method: 'get', params })
}
export function getInspectionRuleGroup(id) {
  return request({ url: `/inspection-rule-groups/${id}`, method: 'get' })
}
export function createInspectionRuleGroup(data) {
  return request({ url: '/inspection-rule-groups', method: 'post', data })
}
export function updateInspectionRuleGroup(id, data) {
  return request({ url: `/inspection-rule-groups/${id}`, method: 'put', data })
}
export function deleteInspectionRuleGroup(id) {
  return request({ url: `/inspection-rule-groups/${id}`, method: 'delete' })
}
export async function getAllInspectionRuleGroups() {
  const groups = []
  for (let page = 1; ; page++) {
    const { data } = await getInspectionRuleGroups({ page, page_size: 100 })
    groups.push(...data.list)
    if (groups.length >= data.total || !data.list.length) return groups
  }
}

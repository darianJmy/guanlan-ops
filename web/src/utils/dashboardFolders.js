export function buildDashboardFolderTree(folders = []) {
  const nodes = new Map(folders.map(folder => [Number(folder.id), { ...folder, id: Number(folder.id), children: [] }]))
  const roots = []
  for (const node of nodes.values()) {
    const parent = nodes.get(Number(node.parent_id))
    if (parent && parent.id !== node.id) parent.children.push(node)
    else roots.push(node)
  }
  const sortNodes = list => {
    list.sort((a, b) => (a.sort ?? 0) - (b.sort ?? 0) || a.name.localeCompare(b.name) || a.id - b.id)
    list.forEach(item => sortNodes(item.children))
  }
  sortNodes(roots)
  return roots
}

export function dashboardFolderSelectTree(folders = []) {
  return [{ id: 0, name: '根目录', children: buildDashboardFolderTree(folders) }]
}

export function dashboardFolderPermissions(scope) {
  return scope === 'services'
    ? { list: 'service-dashboard:list', create: 'service-dashboard:manage', edit: 'service-dashboard:manage', delete: 'service-dashboard:manage' }
    : { list: 'dashboard:list', create: 'dashboard:create', edit: 'dashboard:edit', delete: 'dashboard:delete' }
}

export function dashboardFolderDropId(scope, dataTransfer) {
  if (scope === 'services') {
    const id = dataTransfer?.getData('application/x-guanlan-service-id') || ''
    return /^[A-Za-z0-9_-]{1,128}$/.test(id) ? id : null
  }
  const id = Number(dataTransfer?.getData('application/x-guanlan-dashboard-id'))
  return Number.isInteger(id) && id > 0 ? id : null
}

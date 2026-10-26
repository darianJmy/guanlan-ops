export function datasourceGroupTree(groups) {
  const nodes = new Map(groups.map(group => [group.id, { ...group, children: [] }]))
  const roots = []
  for (const node of nodes.values()) {
    const parent = nodes.get(node.parent_id)
    if (parent && parent !== node) parent.children.push(node)
    else roots.push(node)
  }
  return roots
}

export function filterDatasourceGroupTree(nodes, keyword) {
  const value = keyword.trim().toLowerCase()
  if (!value) return nodes
  return nodes.flatMap(node => {
    if (`${node.path || node.name} ${node.description || ''}`.toLowerCase().includes(value)) return [node]
    const children = filterDatasourceGroupTree(node.children, keyword)
    return children.length ? [{ ...node, children }] : []
  })
}

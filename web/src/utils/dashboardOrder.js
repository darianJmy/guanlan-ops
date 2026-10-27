export const dashboardKey = item => `${item.kind}:${item.id}`

export function moveDashboard(items, key, targetIndex) {
  const result = [...items]
  const from = result.findIndex(item => dashboardKey(item) === key)
  if (from < 0 || targetIndex < 0 || targetIndex >= result.length) return result
  const [item] = result.splice(from, 1)
  result.splice(targetIndex, 0, item)
  return result
}

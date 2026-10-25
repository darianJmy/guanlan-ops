// Keep dotted source keys authoritative; retain arrays and empty objects as values.
export function flattenLogFields(fields) {
  const result = Object.create(null)
  function visit(object, prefix = '') {
    for (const [key, value] of Object.entries(object)) {
      const path = prefix ? `${prefix}.${key}` : key
      if (value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length) visit(value, path)
      else result[path] = value
    }
    for (const [key, value] of Object.entries(object)) {
      if (key.includes('.')) result[prefix ? `${prefix}.${key}` : key] = value
    }
  }
  visit(fields)
  return result
}
export function formatLogField(value) {
  if (value === undefined) return '—'
  if (value === null) return 'null'
  return typeof value === 'object' ? JSON.stringify(value) : String(value)
}

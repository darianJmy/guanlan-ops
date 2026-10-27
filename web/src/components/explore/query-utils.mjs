export const durations = [
  { label: '最近 5 分钟', value: 300 }, { label: '最近 15 分钟', value: 900 },
  { label: '最近 1 小时', value: 3600 }, { label: '最近 6 小时', value: 21600 },
  { label: '最近 24 小时', value: 86400 }, { label: '最近 7 天', value: 604800 },
]
export function queryRange(seconds, custom, now = Date.now()) {
  const dates = seconds ? [new Date(now - seconds * 1000), new Date(now)] : (custom || []).map(value => new Date(value))
  if (dates.length !== 2 || dates.some(date => !Number.isFinite(date.getTime())) || dates[1] <= dates[0] || dates[1] - dates[0] > 31 * 86400000) throw new Error('请选择有效时间范围，最长 31 天')
  return dates
}
export function queryStep(range, configured = 0) {
  const minimum = Math.ceil((range[1] - range[0]) / 1000 / 11000)
  if (configured && configured < minimum) throw new Error(`当前范围的步长至少为 ${minimum} 秒`)
  return configured || Math.max(1, Math.ceil((range[1] - range[0]) / 1000 / 800))
}
export function highlightParts(line, keyword) {
  if (!keyword) return [{ text: line, match: false }]
  const parts = [], haystack = line.toLocaleLowerCase(), needle = keyword.toLocaleLowerCase()
  let cursor = 0, index
  while ((index = haystack.indexOf(needle, cursor)) >= 0) {
    if (index > cursor) parts.push({ text: line.slice(cursor, index), match: false })
    parts.push({ text: line.slice(index, index + keyword.length), match: true }); cursor = index + keyword.length
  }
  if (cursor < line.length) parts.push({ text: line.slice(cursor), match: false })
  return parts
}
export function logLevel(entry) {
  const explicit = entry.labels?.level || entry.labels?.severity || entry.metadata?.level || entry.fields?.['log.level']
  const match = String(explicit || entry.line.match(/\b(trace|debug|info|warn(?:ing)?|error|fatal|critical)\b/i)?.[0] || 'unknown').toLowerCase()
  return match === 'warning' ? 'warn' : match === 'critical' ? 'fatal' : match
}
export function appendLogFilter(query, type, field, value) {
  if (type === 'loki') {
    if (field) {
      if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(field)) throw new Error('此字段不支持直接添加到 LogQL')
      // Add a label-filter pipeline rather than rewriting an arbitrary selector.
      return `${query.trim()} | ${field}=${JSON.stringify(String(value))}`
    }
    return `${query.trim()} |= ${JSON.stringify(String(value))}`
  }
  if (type === 'elasticsearch') {
    if (field && !/^[\w@.\-]+$/.test(field)) throw new Error('此字段名称不支持快捷筛选')
    const phrase = `"${String(value).replace(/([\\"])/g, '\\$1')}"`
    return `(${query.trim() || '*'}) AND ${field ? field + ':' : ''}${phrase}`
  }
  throw new Error('该数据源请直接编辑关键词')
}
export function metricSeries(result, reference) {
  if (result.resultType === 'scalar' || result.resultType === 'string') return [{ name: reference, data: [result.result], metric: {} }]
  return (result.result || []).map(item => ({
    name: `${reference} · ${Object.entries(item.metric || {}).map(([key,value]) => `${key}=${JSON.stringify(value)}`).join(', ') || 'value'}`,
    metric: item.metric || {}, data: item.values || (item.value ? [item.value] : []),
  }))
}
export function decodeQueryState(encoded) {
  try {
    if (!encoded || encoded.length > 12000) return null
    const state = JSON.parse(encoded)
    if (!Array.isArray(state.queries) || !state.queries.length || state.queries.length > 6 || state.queries.some(item => typeof item.text !== 'string' || item.text.length > 65536)) return null
    const duration = Number(state.duration)
    if (!Number.isFinite(duration) || duration < 0 || duration > 31 * 86400) return null
    const customRange = duration ? [] : queryRange(0,state.customRange).map(date => date.toISOString())
    return { queries: state.queries.map(item=>({text:item.text,enabled:item.enabled!==false})), duration, customRange, queryType: state.queryType==='instant'?'instant':'range' }
  } catch { return null }
}

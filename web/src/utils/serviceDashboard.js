export const serviceStatuses = {
  direct: { label: '直连', type: 'info' },
  unknown: { label: '状态未知', type: 'warning' },
  running: { label: '运行中', type: 'success' },
  idle: { label: '待连接', type: 'info' },
  stopped: { label: '已停用', type: 'info' },
  stopping: { label: '停用中', type: 'warning' },
  connecting: { label: '连接中', type: 'warning' },
  retrying: { label: '重连中', type: 'warning' },
  failed: { label: '连接失败', type: 'danger' },
}

export function serviceStatus(item) {
  return item.status || item.resource?.status || 'stopped'
}

export function canEditService(item) {
  return item.resource?.type === 'http_direct' || !item.resource?.desired && ['stopped', 'failed'].includes(serviceStatus(item))
}

export function canManageServiceResource(item, globalManage) {
  const access = item.access_level || (Number(item.resource?.folder_id || 0) ? 'none' : 'manage')
  return !!globalManage && ['edit', 'manage'].includes(access)
}

export function httpTargetURL(value) {
  const invalid = () => { throw new Error('请输入有效的 HTTP 或 HTTPS 完整地址，不能包含用户名和密码') }
  if (typeof value !== 'string' || value.length > 2048 || /[\s\\]/.test(value) || !/^https?:\/\/[^/]/i.test(value)) invalid()
  let url
  try { url = new URL(value) } catch { invalid() }
  const authority = value.match(/^https?:\/\/([^/?#]*)/i)[1]
  if (!['http:', 'https:'].includes(url.protocol) || !url.hostname || url.username || url.password || authority.includes('@') || authority.endsWith(':') || (url.port && (Number(url.port) < 1 || Number(url.port) > 65535))) invalid()
  if (!url.hostname.startsWith('[') && (url.hostname.length > 254 || !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*\.?$/i.test(url.hostname))) invalid()
  let path
  try { path = decodeURIComponent(url.pathname) } catch { invalid() }
  if (path.startsWith('//') || path.startsWith('/_portal/') || /[\r\n\\]/.test(path) || `${url.pathname}${url.search}${url.hash}`.length > 1000) invalid()
  return url.href
}

// A launch URL must be an HTTP(S) ticket exchange, never executable page content.
export function portalAccessURL(value) {
  const url = new URL(value)
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/_portal/access' || !url.searchParams.get('ticket')) {
    throw new Error('服务返回了无效的访问地址')
  }
  return url.href
}

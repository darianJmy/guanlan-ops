export function parseCertificateTarget(value) {
  const raw = String(value || '').trim()
  if (!raw || /[\s/?#@\\]/.test(raw)) throw new Error('请输入有效的网站域名，可附加端口')
  const url = new URL(`https://${raw}`)
  const port = url.port ? Number(url.port) : 443
  if (port < 1 || port > 65535) throw new Error('端口范围为 1 至 65535')
  return { host: url.hostname.replace(/^\[|\]$/g, ''), port }
}

export function formatCertificateTarget(host, port = 443) {
  const value = String(host || '').trim()
  if (!value) return value
  const hostname = value.includes(':') && !value.startsWith('[') ? `[${value}]` : value
  return !port || Number(port) === 443 ? hostname : `${hostname}:${port}`
}

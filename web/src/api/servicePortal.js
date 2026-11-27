import request from './request'

const portal = (path, config = {}) => request({ url: `/service-portal${path}`, timeout: 45000, ...config })
const itemPath = (type, id) => `/${type}/${encodeURIComponent(id)}`

export const getServicePortalInfo = (config = {}) => portal('/me', config)
export const getPortalResources = (config = {}) => portal('/resources', config)
export const savePortalResource = (data) => portal('/resources', { method: 'post', data })
export const deletePortalResource = (id) => portal(itemPath('resources', id), { method: 'delete' })
export const setPortalResourceRunning = (id, running) => portal(`${itemPath('resources', id)}/${running ? 'start' : 'stop'}`, { method: 'post', timeout: 60000 })
export const createPortalAccess = (id) => portal(`${itemPath('resources', id)}/access`, { method: 'post' })
export const movePortalResource = (id, folderId) => portal(`${itemPath('resources', id)}/folder`, { method: 'post', data: { folder_id: folderId } })
// Kubernetes connections are managed in 观澜 接入中心. The service page only
// consumes the runtime discovery APIs below; it never uploads or stores a
// kubeconfig itself.
export const getPortalNamespaces = (id) => portal(`${itemPath('clusters', id)}/namespaces`)
export const getPortalServices = (id, namespace) => portal(`${itemPath('clusters', id)}/services`, { params: { namespace } })

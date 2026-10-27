import request from './request'

export function promInstantQuery(data) { return request({ url: '/prometheus/query', method: 'post', data }) }
export function promRangeQuery(data) { return request({ url: '/prometheus/query_range', method: 'post', data }) }
export function promMetadata(params, options = {}) { return request({ ...options, url: '/prometheus/metadata', method: 'get', params }) }
export function getPromDatasources() { return request({ url: '/prometheus/datasources', method: 'get' }) }
export function getPromQueryHistory() { return request({ url: '/prometheus/query-history', method: 'get' }) }
export function clearPromQueryHistory() { return request({ url: '/prometheus/query-history', method: 'delete' }) }

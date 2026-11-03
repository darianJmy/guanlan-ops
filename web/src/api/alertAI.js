import request from './request'
const base = id => `/fault-centers/${id}`
export const getAlertAIRules = id => request({ url: `${base(id)}/ai-analysis-rules` })
export const saveAlertAIRule = (id, ruleId, data) => request({ url: `${base(id)}/ai-analysis-rules${ruleId ? `/${ruleId}` : ''}`, method: ruleId ? 'put' : 'post', data })
export const deleteAlertAIRule = (id, ruleId) => request({ url: `${base(id)}/ai-analysis-rules/${ruleId}`, method: 'delete' })
export const getAlertAIOptions = id => request({ url: `${base(id)}/ai-analysis-options` })
export const getAlertAIAnalysis = (id, alertId) => request({ url: `${base(id)}/alerts/${alertId}/ai-analysis` })


<template>
  <div class="span-detail" @click.stop>
    <header><h3>{{ span.operationName || '未命名操作' }}</h3></header>
    <div class="summary">
      <span><i class="service-mark"></i>服务：<strong>{{ process.serviceName || '—' }}</strong></span>
      <span>耗时：<strong>{{ formatDuration(span.duration) }}</strong></span>
      <span>开始时间：<strong>{{ formatDuration(span.startTime - traceStart) }}</strong>（{{ startTime }}）</span>
    </div>
    <div class="attribute-columns">
      <details v-for="section in sections" :key="section.title" class="attribute-section" open>
        <summary>{{ section.title }}<span class="count">{{ section.tags.length }}</span></summary>
        <div class="section-body">
          <p v-if="!section.tags.length" class="empty-attributes">暂无属性</p>
          <details v-for="group in groupAttributes(section.tags)" :key="group.name" class="attribute-group" open>
            <summary>{{ group.name }}<span class="count">{{ group.items.length }}</span></summary>
            <table><tbody><tr v-for="(item, index) in group.items" :key="index">
              <th scope="row">{{ item.key }}</th>
              <td><code :class="valueType(item)">{{ formatValue(item.value) }}</code></td>
              <td class="copy-cell"><el-button text :icon="CopyDocument" :aria-label="`复制 ${item.key} 的值`" title="复制值" @click="copyValue(item.value)" /></td>
            </tr></tbody></table>
          </details>
        </div>
      </details>
    </div>
    <footer><span>Span ID：<code>{{ span.spanID }}</code><el-button text :icon="CopyDocument" aria-label="复制 Span ID" @click="copyValue(span.spanID)" /></span><span>Trace ID：<code>{{ traceId }}</code><el-button text :icon="CopyDocument" aria-label="复制 Trace ID" @click="copyValue(traceId)" /></span></footer>
  </div>
</template>
<script setup>
import { computed } from 'vue'
import { ElMessage } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
const props = defineProps({ span: { type: Object, required: true }, process: { type: Object, default: () => ({}) }, traceStart: { type: Number, required: true }, traceId: { type: String, required: true } })
const sections = computed(() => [{ title: 'Span 属性', tags: props.span.tags || [] }, { title: '资源属性', tags: props.process.tags || [] }])
const startTime = computed(() => {
  const date = new Date(props.span.startTime / 1000)
  return date.toLocaleTimeString('zh-CN', { hour12: false }) + '.' + String(date.getMilliseconds()).padStart(3, '0')
})
function formatDuration(value) {
  if (!Number.isFinite(value)) return '—'
  return Math.abs(value) < 1000 ? `${value} μs` : Math.abs(value) < 1000000 ? `${Number((value / 1000).toFixed(3))} ms` : `${Number((value / 1000000).toFixed(3))} s`
}
function groupAttributes(tags) {
  const groups = new Map()
  for (const item of tags) {
    const key = String(item.key || '')
    const name = /^http\./.test(key) ? 'HTTP' : /^url\./.test(key) ? 'URL' : /^(network|net|client|server)\./.test(key) ? 'Network' : /^(host|os)\./.test(key) ? 'Host / OS' : /^telemetry\./.test(key) ? 'Telemetry SDK' : /^(db|database)\./.test(key) ? 'Database' : /^(service|process)\./.test(key) ? 'Service / Process' : '其他'
    if (!groups.has(name)) groups.set(name, [])
    groups.get(name).push(item)
  }
  return [...groups].map(([name, items]) => ({ name, items }))
}
const formatValue = value => JSON.stringify(value) ?? '—'
const valueType = item => ['int64', 'float64', 'number'].includes(item.type) || typeof item.value === 'number' ? 'value-number' : typeof item.value === 'boolean' ? 'value-boolean' : 'value-string'
async function copyValue(value) {
  const text = typeof value === 'string' ? value : formatValue(value)
  try {
    if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(text)
    else {
      const input = document.createElement('textarea')
      input.value = text; input.style.position = 'fixed'; input.style.opacity = '0'; document.body.appendChild(input)
      try { input.select(); if (!document.execCommand('copy')) throw new Error('copy failed') } finally { input.remove() }
    }
    ElMessage.success('已复制')
  } catch { ElMessage.error('复制失败，请手动选择并复制') }
}
</script>
<style scoped>
.span-detail{padding:18px 22px;border-top:3px solid var(--ops-primary);background:var(--ops-surface-soft);color:var(--ops-text)}
h3{margin:0;font-size:15px;line-height:1.6;overflow-wrap:anywhere}.summary{display:flex;flex-wrap:wrap;gap:12px 24px;margin:10px 0 18px;color:var(--ops-muted);font-size:12px}.summary>span{display:flex;align-items:center;gap:4px;flex-wrap:wrap}.summary strong{color:var(--ops-text);font-weight:500}.service-mark{height:14px;width:3px;background:var(--ops-primary);margin-right:3px}
.attribute-columns{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px;align-items:start}.attribute-section{min-width:0;border:1px solid var(--ops-border);background:var(--ops-surface)}summary{cursor:pointer;user-select:none;font-weight:600;font-size:12px;padding:10px 12px}summary:focus-visible{outline:2px solid var(--ops-primary);outline-offset:-2px}.count{margin-left:8px;color:var(--ops-muted);font-size:11px;font-weight:400;background:var(--ops-surface-soft);padding:1px 6px;border-radius:3px}.section-body{border-top:1px solid var(--ops-border-soft)}.attribute-group+ .attribute-group{border-top:1px solid var(--ops-border-soft)}.attribute-group>summary{font-weight:500;background:var(--ops-surface-soft)}table{width:100%;border-collapse:collapse;table-layout:fixed;font-size:12px}th,td{padding:8px 10px;vertical-align:top;border-top:1px solid var(--ops-border-soft);text-align:left;overflow-wrap:anywhere}th{width:39%;font-weight:400;color:var(--ops-muted)}code{white-space:pre-wrap;overflow-wrap:anywhere;font:12px/1.6 Consolas,monospace}.copy-cell{width:30px;padding:3px 4px}.copy-cell .el-button{padding:6px}.value-string{color:var(--ops-success)}.value-number{color:var(--ops-primary)}.value-boolean{color:var(--ops-warning)}.empty-attributes{padding:0 12px;color:var(--ops-muted);font-size:12px}footer{display:flex;gap:12px 24px;flex-wrap:wrap;margin-top:14px;color:var(--ops-muted);font-size:11px}footer>span{display:flex;align-items:center;gap:4px;min-width:0;flex-wrap:wrap}footer .el-button{padding:4px}
@media(max-width:1000px){.attribute-columns{grid-template-columns:minmax(0,1fr)}}@media(max-width:600px){.span-detail{padding:12px}}
</style>

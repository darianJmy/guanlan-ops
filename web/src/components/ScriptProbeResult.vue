<template>
  <div class="script-probe-result">
    <div class="script-result-meta">
      <strong>{{ detail.automation_task_name || detail.script_name || '自动化任务' }}</strong>
      <el-tag :type="detail.exit_code === 0 && !detail.error ? 'success' : 'danger'" size="small">退出码 {{ detail.exit_code ?? '—' }}</el-tag>
    </div>
    <p v-if="detail.error" class="script-result-error">{{ detail.error }}</p>
    <pre>{{ detail.output || '（无输出）' }}</pre>
    <small v-if="detail.truncated">输出过长，已截断展示。</small>
  </div>
</template>
<script setup>
import { computed } from 'vue'
const props = defineProps({ result: { type: [String, Object], default: '' } })
const detail = computed(() => {
  if (props.result && typeof props.result === 'object') return props.result
  try { return JSON.parse(props.result || '{}') || {} } catch { return { error: props.result } }
})
</script>
<style scoped>
.script-probe-result{min-width:0;text-align:left}.script-result-meta{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.script-result-meta strong{color:var(--ops-text);font-size:13px}.script-result-error{color:var(--el-color-danger);white-space:pre-wrap;overflow-wrap:anywhere}.script-probe-result pre{max-height:280px;margin:10px 0 0;padding:12px;overflow:auto;color:var(--ops-text);background:var(--ops-surface-soft);border:1px solid var(--ops-border);border-radius:6px;font:12px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap;overflow-wrap:anywhere}.script-probe-result small{color:var(--ops-muted)}
</style>

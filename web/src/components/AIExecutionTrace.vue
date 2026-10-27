<template>
  <details v-if="execution" class="ai-execution-trace">
    <summary>执行详情 · {{ statusLabel(execution.status) }} · {{ formatDuration(execution.duration_ms) }}</summary>
    <div class="trace-id">任务编号：{{ execution.run_id }}</div>
    <ol>
      <li v-for="(step, index) in execution.steps" :key="index">
        <span>第 {{ step.round }} 轮 · {{ step.kind === 'model' ? '模型分析' : (toolLabels[step.tool] || '工具校验') }}</span>
        <span>{{ statusLabel(step.status) }} · {{ formatDuration(step.duration_ms) }}</span>
        <small v-if="step.error_code">{{ errorLabels[step.error_code] || '调用失败' }}</small>
      </li>
    </ol>
    <p v-if="!execution.steps.length">本次任务未执行工具或模型调用。</p>
  </details>
</template>

<script setup>
import { computed } from 'vue'
const props = defineProps({ metadata: { type: [String, Object], default: '' } })
const execution = computed(() => {
  try {
    const value = typeof props.metadata === 'string' ? JSON.parse(props.metadata || '{}') : props.metadata
    const trace = value?.execution
    return trace && Array.isArray(trace.steps) ? trace : null
  } catch { return null }
})
const toolLabels = {
  query_prometheus_instant: '查询指标', list_k8s_resources: '查询 Kubernetes 资源',
  find_k8s_pod_anomalies: '检查异常 Pod', list_sls_logstores: '查询 SLS 日志库',
  query_tencent_cls: '检索 CLS 日志', list_nacos_services: '查询 Nacos 服务',
  list_kafka_topics: '查询 Kafka 主题', list_kafka_consumer_lag: '查询 Kafka 消费延迟',
}
const errorLabels = {
  MODEL_ERROR: '模型服务调用失败', INVALID_MODEL_RESPONSE: '模型返回格式无效',
  UNKNOWN_TOOL: '工具未注册', FORBIDDEN: '没有操作权限', INVALID_PARAMS: '工具参数不符合要求',
  READ_ONLY_REQUIRED: '当前仅支持只读操作', TOOL_TIMEOUT: '工具超时或已取消', CANCELLED: '调用已停止',
}
function statusLabel(status) {
  return { succeeded: '完成', failed: '失败', cancelled: '已停止', timeout: '超时', limited: '达到调用上限' }[status] || '未知状态'
}
function formatDuration(value) { return `${(Math.max(0, Number(value) || 0) / 1000).toFixed(2)} 秒` }
</script>

<style scoped>
.ai-execution-trace{margin-top:8px;padding:8px 10px;border:1px solid var(--ops-border,var(--ops-border));border-radius:6px;font-size:12px;color:var(--ops-muted,var(--ops-muted));background:var(--ops-surface,#fff)}
summary{cursor:pointer}.trace-id{margin-top:8px;overflow-wrap:anywhere;font-size:11px}ol{padding-left:20px;margin-bottom:0}li{padding:5px 0}li span{display:block}small{color:var(--el-color-danger,var(--ops-danger))}
</style>

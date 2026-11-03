<template>
  <section class="ai-analysis-result">
    <div class="result-heading"><strong>AI 分析</strong><el-button link type="primary" :loading="loading" @click="load">刷新</el-button></div>
    <p class="result-note">后台只读分析，告警通知不等待 AI。</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <p v-else-if="!current">{{ loading ? '正在读取分析状态…' : emptyNotice }}</p>
    <article v-for="item in displayed" :key="item.job.id" class="analysis-report">
      <div class="result-heading"><el-tag :type="statusType(item.job.status)">{{ statusText(item.job.status) }}</el-tag><small v-if="item.job.episode_key !== episodeKey">历史轮次</small></div>
      <p class="result-note">{{ item.job.model_name || '模型不可用' }} · {{ item.job.rule_name }}<br />开始：{{ formatTime(item.job.started_at) }}<template v-if="item.job.finished_at"><br />结束：{{ formatTime(item.job.finished_at) }}</template></p>
      <p v-if="item.job.error_message">{{ item.job.error_message }}</p>
      <template v-if="item.result">
        <p class="result-summary">{{ item.result.summary }}</p>
        <template v-if="item.result.possible_causes?.length"><h4>可能原因</h4><ul><li v-for="(cause, i) in item.result.possible_causes" :key="i">{{ cause.text }} <small>{{ cause.evidence_ids?.join('、') }}</small></li></ul></template>
        <template v-if="item.result.suggestions?.length"><h4>处理建议</h4><ul><li v-for="(value, i) in item.result.suggestions" :key="i">{{ value }}</li></ul></template>
        <template v-if="item.result.missing_information?.length"><h4>缺失信息</h4><ul><li v-for="(value, i) in item.result.missing_information" :key="i">{{ value }}</li></ul></template>
      </template>
      <details v-if="item.evidence?.length"><summary>查询依据（{{ item.evidence.length }}）</summary><article v-for="e in item.evidence" :key="e.id" class="evidence"><strong>{{ e.id }} · {{ e.datasource_name || '关联告警' }}</strong><p>{{ e.summary }}</p><code v-if="e.query">{{ e.query }}</code><p class="result-note">{{ formatTime(e.range_start) }} — {{ formatTime(e.range_end) }}</p><details v-if="e.data"><summary>查看查询数据</summary><pre>{{ JSON.stringify(e.data, null, 2) }}</pre></details></article></details>
    </article>
    <el-button v-if="analyses.length > (current ? 1 : 0)" link type="primary" @click="showHistory = !showHistory">{{ showHistory ? '收起历史分析' : '查看历史分析' }}</el-button>
  </section>
</template>
<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { getAlertAIAnalysis } from '../api/alertAI'
const props = defineProps({ centerId: { type: Number, required: true }, alertId: { type: Number, required: true } })
const analyses = ref([]), episodeKey = ref(''), loading = ref(false), error = ref(''), showHistory = ref(false), emptyNotice = ref('尚无分析记录')
const current = computed(() => analyses.value.find(item => item.job.episode_key === episodeKey.value))
const displayed = computed(() => showHistory.value ? analyses.value : current.value ? [current.value] : [])
let timer, generation = 0, stopped = false
const statusText = value => ({ queued: '等待分析', running: '分析中', succeeded: '已完成', insufficient: '证据不足', failed: '失败', timeout: '超时', cancelled: '已取消', skipped: '已跳过' }[value] || value)
const statusType = value => ({ succeeded: 'success', failed: 'danger', timeout: 'warning', insufficient: 'warning' }[value] || 'info')
const formatTime = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'
async function load() {
  clearTimeout(timer)
  const version = ++generation
  loading.value = true; error.value = ''
  try { const data = (await getAlertAIAnalysis(props.centerId, props.alertId)).data || {}; if (stopped || version !== generation) return; analyses.value = data.analyses || []; episodeKey.value = data.episode_key || ''; emptyNotice.value = data.empty_notice || '尚无分析记录'; if (!current.value || ['queued', 'running'].includes(current.value.job.status)) timer = setTimeout(load, 5000) }
  catch { if (version === generation) error.value = '读取分析失败，请刷新重试' }
  finally { if (version === generation) loading.value = false }
}
watch(() => [props.centerId, props.alertId], () => { analyses.value = []; showHistory.value = false; load() }, { immediate: true })
onBeforeUnmount(() => { stopped = true; generation++; clearTimeout(timer) })
</script>
<style scoped>
.ai-analysis-result{margin-top:20px;padding-top:16px;border-top:1px solid var(--ops-border);color:var(--ops-text);font-size:12px;line-height:1.7;overflow-wrap:anywhere}
.result-heading{display:flex;align-items:center;justify-content:space-between;gap:8px}.result-heading strong{font-size:14px}.result-note,small{color:var(--ops-muted)}
.analysis-report{padding:12px 0}.analysis-report+.analysis-report{border-top:1px solid var(--ops-border)}.result-summary{white-space:pre-wrap}h4{margin:12px 0 4px;font-size:12px}ul{padding-left:20px;margin:4px 0}summary{cursor:pointer;color:var(--ops-primary)}.evidence{margin-top:12px;padding:12px;background:var(--ops-surface-soft)}code{display:block;white-space:pre-wrap;font-size:11px}pre{max-height:260px;overflow:auto;white-space:pre-wrap;font-size:11px}
</style>

<template>
    <el-drawer v-model="historyVisible" class="query-history-drawer" size="min(420px, 92vw)" style="border-radius: 0" append-to-body>
      <template #header>
        <div class="history-drawer-header">
          <div class="standard-drawer-header"><DrawerBackButton @click="historyVisible = false" /><strong>历史查询</strong></div>
          <el-button v-if="queryHistory.length" text type="danger" @click="clearQueryHistory">清空</el-button>
        </div>
      </template>
      <p v-if="loading">正在加载…</p>
      <div v-else-if="queryHistory.length" class="history-list">
        <button v-for="item in queryHistory" :key="item.id" class="history-item" type="button" @click="restoreQuery(item)">
          <span class="history-query">{{ item.query }}</span>

        </button>
      </div>
      <el-empty v-else description="暂无查询记录" :image-size="72" />
    </el-drawer>
</template>
<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import DrawerBackButton from '../DrawerBackButton.vue'
import { getPromQueryHistory, clearPromQueryHistory } from '../../api/prometheus'
const emit = defineEmits(['select'])
const historyVisible = ref(false), queryHistory = ref([]), loading = ref(false)
async function openQueryHistory() {
  historyVisible.value = true
  queryHistory.value = []
  loading.value = true
  try { const response = await getPromQueryHistory(); queryHistory.value = response.data || [] }
  catch { ElMessage.error('读取历史查询失败，请重试') }
  finally { loading.value = false }
}
function restoreQuery(item) { emit('select', item); historyVisible.value = false }
async function clearQueryHistory() {
  try { await clearPromQueryHistory(); queryHistory.value = []; ElMessage.success('历史记录已清空') }
  catch { ElMessage.error('清空历史查询失败，请重试') }
}
defineExpose({ openQueryHistory })
</script>
<style scoped>
:global(.query-history-drawer) {
  width: min(420px, 92vw) !important;
  border-radius: 0 !important;
}
.history-drawer-header {
  display: flex;
  width: 100%;
  align-items: center;
  justify-content: space-between;
}
.history-drawer-header strong {
  color: var(--ops-text);
  font-size: 16px;
}
.history-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.history-item {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: 9px;
  padding: 12px 14px;
  color: var(--ops-text);
  text-align: left;
  background: var(--ops-surface);
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  cursor: pointer;
  transition: border-color .16s ease, background .16s ease, box-shadow .16s ease;
}
.history-item:hover,
.history-item:focus-visible {
  outline: none;
  border-color: #9fc8f7;
  background: var(--ops-primary-soft);
  box-shadow: 0 4px 12px rgba(64, 158, 255, .08);
}
.history-query {
  overflow: hidden;
  font: 13px/1.5 'SF Mono', 'Monaco', 'Menlo', 'Consolas', monospace;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>

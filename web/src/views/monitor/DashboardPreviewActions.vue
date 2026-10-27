<template>
  <div class="dashboard-preview-actions">
    <el-button size="small" class="refresh-button" :loading="loading" @click="$emit('refresh')">刷新</el-button>
    <a v-if="url" class="open-link" :href="url" target="_blank" rel="noopener noreferrer">新窗口打开</a>
    <span v-else class="open-link is-disabled" aria-disabled="true">新窗口打开</span>
  </div>
</template>

<script setup>
defineProps({ url: { type: String, default: '' }, loading: Boolean })
defineEmits(['refresh'])
</script>

<style scoped>
.dashboard-preview-actions { display: flex; align-items: center; gap: 10px; }
.refresh-button { --el-button-bg-color: var(--ops-surface-soft); --el-button-border-color: transparent; --el-button-text-color: var(--ops-text); --el-button-hover-bg-color: var(--el-fill-color-dark); --el-button-hover-border-color: transparent; --el-button-hover-text-color: var(--ops-text); height: 26px; padding: 0 10px; font-size: 12px; font-weight: 400; }
.open-link { color: var(--ops-primary); font-size: 12px; line-height: 1.5; text-decoration: underline; text-underline-offset: 3px; white-space: nowrap; }
.open-link:hover { color: var(--el-color-primary-dark-2); }
.open-link:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 3px; border-radius: 2px; }
.open-link.is-disabled { color: var(--ops-subtle); cursor: default; }
</style>

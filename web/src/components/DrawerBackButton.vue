<template>
  <el-button text :icon="ArrowLeft" class="back-button drawer-back-button"><slot>返回</slot></el-button>
</template>

<script setup>
import { ArrowLeft } from '@element-plus/icons-vue'
</script>

<style scoped>
.el-button.back-button.drawer-back-button {
  flex-shrink: 0;
  height: 32px;
  margin: 0;
  padding: 0 8px;
  color: var(--ops-primary);
  font-size: 14px;
  font-weight: 400;
}
</style>

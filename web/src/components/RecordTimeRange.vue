<template>
  <div class="record-time-range-container">
  <el-date-picker
    class="record-time-range"
    :model-value="modelValue"
    :type="dateOnly ? 'daterange' : 'datetimerange'"
    :format="dateOnly ? 'YYYY-MM-DD' : 'YYYY-MM-DD HH:mm'"
    :value-format="dateOnly ? 'YYYY-MM-DD' : 'YYYY-MM-DD HH:mm:ss'"
    :start-placeholder="dateOnly ? '开始日期' : '开始时间'"
    :end-placeholder="dateOnly ? '结束日期' : '结束时间'"
    range-separator="至"
    size="default"
    clearable
    @update:model-value="$emit('update:modelValue', $event)"
    @change="$emit('change', $event)"
  />
  </div>
</template>

<script setup>
defineProps({ modelValue: { type: Array, default: () => [] }, dateOnly: Boolean })
defineEmits(['update:modelValue', 'change'])
</script>

<style>
.record-time-range-container {
  box-sizing: border-box;
  width: 250px;
  max-width: 100%;
  min-width: 0;
  flex: 0 0 250px;
}
.record-time-range-container .record-time-range.el-date-editor.el-range-editor {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  height: 32px;
  padding: 0 10px;
  border-radius: var(--ops-radius-sm);
}
.record-time-range .el-range-input { min-width: 0; font-size: 12px; }
.record-time-range .el-range-separator { flex: 0 0 24px; padding: 0; font-size: 12px; color: var(--ops-muted); }
@media (max-width: 760px) {
  .record-time-range-container { width: 100%; flex-basis: 100%; }
}
</style>

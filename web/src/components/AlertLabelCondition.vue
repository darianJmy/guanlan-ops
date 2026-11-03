<template>
  <div class="label-conditions">
    <div v-for="(item, index) in modelValue" :key="index" class="condition-row">
      <div class="condition-fields" :class="{ 'key-only': keyOnly }">
        <el-input :model-value="item.key" placeholder="Key" maxlength="64" :aria-label="`${label} ${index + 1} Key`" @update:model-value="update(index, 'key', $event)" />
        <el-input v-if="!keyOnly" :model-value="item.value" placeholder="Value" maxlength="255" :aria-label="`${label} ${index + 1} Value`" @update:model-value="update(index, 'value', $event)" />
      </div>
      <el-button class="condition-remove" text :icon="Delete" :disabled="modelValue.length <= minItems" :aria-label="`删除${label} ${index + 1}`" title="删除条件" @click="remove(index)" />
    </div>
    <el-button class="add-condition" plain :icon="Plus" :disabled="modelValue.length >= 10" @click="addCondition">{{ addLabel || (keyOnly ? '添加标签' : '添加条件') }}</el-button>
  </div>
</template>

<script setup>
import { Delete, Plus } from '@element-plus/icons-vue'
const props = defineProps({
  modelValue: { type: Array, required: true },
  minItems: { type: Number, default: 1 },
  keyOnly: Boolean,
  addLabel: { type: String, default: '' },
  label: { type: String, default: '匹配条件' },
})
const emit = defineEmits(['update:modelValue'])
function update(index, field, value) {
  // The editor uses one merged label set for every new or edited condition.
  emit('update:modelValue', props.modelValue.map((item, i) => i === index ? { ...item, [field]: value, namespace: 'all_labels' } : item))
}
function addCondition() {
  if (props.modelValue.length >= 10) return
  const item = props.keyOnly ? { namespace: 'all_labels', key: '' } : { namespace: 'all_labels', key: '', value: '' }
  emit('update:modelValue', [...props.modelValue, item])
}
function remove(index) {
  if (props.modelValue.length <= props.minItems) return
  emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
}
</script>

<style scoped>
.label-conditions{display:flex;width:100%;min-width:0;flex-direction:column;gap:10px}
.condition-row{display:flex;align-items:center;gap:4px}
.condition-fields{display:grid;min-width:0;flex:1;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:8px}
.condition-fields.key-only{grid-template-columns:minmax(0,1fr)}
.condition-fields :deep(.el-select),.condition-fields :deep(.el-input){width:100%;min-width:0}
.condition-remove{width:24px;height:28px;padding:0;color:var(--ops-muted);flex:0 0 24px}
.condition-remove:not(.is-disabled):hover{color:var(--el-color-danger)}
.add-condition{width:100%;border-style:dashed}
@media(max-width:380px){.condition-fields{grid-template-columns:1fr}}
</style>

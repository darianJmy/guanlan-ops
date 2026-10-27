<template>
  <article class="model-editor" :class="{ 'is-editing': form.id }">
    <div class="editor-heading">
      <strong>{{ form.id ? '编辑模型' : '添加模型' }}</strong>
      <el-button link :icon="Close" :disabled="saving" aria-label="关闭模型表单" @click="emit('cancel')" />
    </div>
    <el-form label-position="top" :disabled="saving" @submit.prevent="emit('save')">
      <el-form-item label="显示名称" required><el-input v-model="form.name" maxlength="128" /></el-form-item>
      <div class="two-column">
        <el-form-item label="Provider" required><el-select v-model="form.provider"><el-option label="NewAPI" value="newapi" /><el-option label="OpenAI 兼容" value="openai" /><el-option label="Anthropic" value="anthropic" /></el-select></el-form-item>
        <el-form-item label="服务地址" required><el-input v-model="form.base_url" /></el-form-item>
      </div>
      <el-form-item label="模型标识" required><el-input v-model="form.model" maxlength="200" /></el-form-item>
      <el-form-item label="API Key" :required="!form.id">
        <el-input v-model="form.api_key" type="password" show-password autocomplete="new-password" />
      </el-form-item>
      <div class="auth-note">
        <el-icon aria-hidden="true"><InfoFilled /></el-icon>
        <span>{{ form.id ? '保存后页面不再显示 API Key，留空保留现有密钥。' : '保存后页面不再显示 API Key。' }}</span>
      </div>
      <el-form-item label="启用"><el-switch v-model="form.enabled" /></el-form-item>
      <div class="editor-footer">
        <div><el-button v-if="form.id" v-permission="['system-setting:edit']" type="danger" plain @click="emit('delete')">删除</el-button></div>
        <div><el-button @click="emit('cancel')">取消</el-button><el-button v-permission="['system-setting:edit']" type="primary" native-type="submit" :loading="saving">保存</el-button></div>
      </div>
    </el-form>
  </article>
</template>

<script setup>
import { Close, InfoFilled } from '@element-plus/icons-vue'
defineProps({ form: { type: Object, required: true }, saving: Boolean })
const emit = defineEmits(['cancel', 'delete', 'save'])
</script>

<style scoped>
.model-editor{margin:0 0 12px;background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 45%,var(--ops-border))}
.model-editor.is-editing{margin:-8px 0 8px;border:1px solid var(--ops-primary);border-top:0}
.editor-heading{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:16px 18px;border-bottom:1px solid var(--ops-border-soft)}
.editor-heading strong{color:var(--ops-text);font-size:15px}
.model-editor :deep(.el-form){padding:20px 22px}.two-column{display:grid;grid-template-columns:1fr 2fr;gap:18px;min-width:0}.two-column>*{min-width:0}
.auth-note{display:flex;align-items:flex-start;gap:8px;margin:-4px 0 18px;padding:9px 11px;color:var(--ops-muted);background:color-mix(in srgb,var(--ops-primary) 6%,transparent);font-size:11px;line-height:1.55}
.auth-note .el-icon{margin-top:2px;flex:none}
.editor-footer{display:flex;align-items:center;justify-content:space-between;gap:12px;padding-top:16px;border-top:1px solid var(--ops-border-soft)}
:deep(.el-select){width:100%}@media(max-width:760px){.two-column{grid-template-columns:1fr;gap:0}.model-editor :deep(.el-form){padding:16px}.editor-footer{align-items:flex-end}}
.auth-note .el-icon,.config-note .el-icon{color:var(--ops-primary)}
</style>

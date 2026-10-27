<template>
  <article class="account-editor" :class="{ 'is-editing': form.id }">
    <div class="editor-heading">
      <div><strong>{{ form.id ? '编辑 Git 认证' : '添加 Git 认证' }}</strong></div>
      <el-button link :icon="Close" aria-label="关闭认证表单" @click="emit('cancel')" />
    </div>
    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" @submit.prevent="submit">
      <el-form-item label="认证名称" prop="name"><el-input v-model="form.name" maxlength="128" /></el-form-item>
      <div class="two-column">
        <el-form-item label="Git 服务" prop="provider">
          <el-select v-model="form.provider"><el-option label="GitHub" value="github" /><el-option label="GitLab" value="gitlab" /><el-option label="Gitee" value="gitee" /><el-option label="其他" value="other" /></el-select>
        </el-form-item>
        <el-form-item label="服务地址" prop="base_url"><el-input v-model="form.base_url" /></el-form-item>
      </div>
      <el-form-item label="认证方式" prop="auth_type">
        <el-radio-group v-model="form.auth_type" class="auth-type-group">
          <el-radio-button value="token">用户名 / Token</el-radio-button>
          <el-radio-button value="none">匿名访问</el-radio-button>
          <el-radio-button value="ssh_key">SSH 私钥</el-radio-button>
        </el-radio-group>
      </el-form-item>
      <div class="auth-note"><el-icon><InfoFilled /></el-icon><span>{{ authNote }}</span></div>
      <div v-if="form.auth_type === 'token'" class="two-column">
        <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
        <el-form-item :label="form.secret_configured ? 'Token / 密码（不修改请留空）' : 'Token / 密码'" :required="!form.secret_configured"><el-input v-model="form.secret" type="password" show-password autocomplete="new-password" /></el-form-item>
      </div>
      <el-form-item v-if="form.auth_type === 'ssh_key'" :label="form.secret_configured ? 'SSH 私钥（不修改请留空）' : 'SSH 私钥'" :required="!form.secret_configured"><el-input v-model="form.secret" type="textarea" :rows="7" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" /></el-form-item>
      <el-form-item label="描述"><el-input v-model="form.description" type="textarea" :rows="2" maxlength="500" /></el-form-item>
      <div class="editor-footer">
        <div><el-button v-if="form.id" type="danger" plain @click="emit('delete')">删除</el-button></div>
        <div><el-button @click="emit('cancel')">取消</el-button><el-button type="primary" native-type="submit" :loading="saving">保存</el-button></div>
      </div>
    </el-form>
  </article>
</template>

<script setup>
import { computed, ref } from 'vue'
import { Close, InfoFilled } from '@element-plus/icons-vue'

const props = defineProps({ form:{ type:Object, required:true }, rules:{ type:Object, required:true }, saving:Boolean })
const emit = defineEmits(['cancel', 'delete', 'save'])
const formRef = ref()
const authNote = computed(() => props.form.auth_type === 'none' ? '匿名认证只能访问公开仓库。' : props.form.auth_type === 'ssh_key' ? 'SSH 私钥仅用于平台拉取仓库，私钥内容不会返回到页面。' : '用户名和 Token 通过 HTTPS 拉取仓库，凭据内容不会返回到页面。')

async function submit() {
  if (!await formRef.value.validate().catch(() => false)) return
  emit('save')
}
</script>

<style scoped>
.account-editor.account-editor{margin:-8px 0 8px;background:var(--ops-surface);border:1px solid var(--ops-primary);border-top:0}.account-editor.account-editor:not(.is-editing){margin:0 0 12px;border-top:1px dashed color-mix(in srgb,var(--ops-primary) 45%,var(--ops-border));border-right-style:dashed;border-bottom-style:dashed;border-left-style:dashed}.editor-heading{display:flex;align-items:center;justify-content:space-between;padding:16px 18px;border-bottom:1px solid var(--ops-border-soft)}.editor-heading>div{display:flex;flex-direction:column;gap:4px}.editor-heading strong{color:var(--ops-text);font-size:15px}.editor-heading span{color:var(--ops-muted);font-size:11px}.account-editor :deep(.el-form){padding:20px 22px}.two-column{display:grid;grid-template-columns:1fr 2fr;gap:18px}.auth-type-group{display:grid;width:100%;grid-template-columns:repeat(3,1fr)}.auth-type-group :deep(.el-radio-button),.auth-type-group :deep(.el-radio-button__inner){width:100%}.auth-note{display:flex;align-items:flex-start;gap:8px;margin:-4px 0 18px;padding:9px 11px;color:var(--ops-muted);background:color-mix(in srgb,var(--ops-primary) 6%,transparent);font-size:11px;line-height:1.55}.auth-note .el-icon{margin-top:2px;flex:none}.editor-footer{display:flex;align-items:center;justify-content:space-between;padding-top:16px;border-top:1px solid var(--ops-border-soft)}:deep(.el-select){width:100%}@media(max-width:760px){.two-column{grid-template-columns:1fr}.editor-footer{align-items:flex-end;gap:14px}}
.auth-note .el-icon,.config-note .el-icon{color:var(--ops-primary)}
</style>

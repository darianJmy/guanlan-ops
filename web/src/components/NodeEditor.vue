<template>
  <article class="node-editor" :class="{ 'is-editing': form.id }">
    <div class="editor-heading"><strong>{{ form.id ? '编辑节点' : '添加节点' }}</strong><el-button link :icon="Close" :disabled="saving" aria-label="关闭节点表单" @click="emit('cancel')" /></div>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="editor-form" :disabled="saving" @submit.prevent="submit">
        <el-form-item label="节点类型" prop="type">
          <div class="type-card-list">
            <button
              v-for="item in nodeTypes"
              :key="item.value"
              class="type-card"
              :class="{ active: form.type === item.value, disabled: form.id && form.type !== item.value }"
              type="button"
              :disabled="!!form.id"
              @click="emit('select-type', item.value)"
            >
              <span class="type-icon"><el-icon><component :is="item.icon" /></el-icon></span>
              <span class="type-content">
                <strong>{{ item.label }}</strong>
              </span>
            </button>
          </div>
        </el-form-item>

        <el-form-item label="节点名称" prop="name">
          <el-input v-model="form.name" />
        </el-form-item>

        <template v-if="form.type === 'ssh'">
          <div class="form-grid">
            <el-form-item label="主机地址" prop="host">
              <el-input v-model="form.host" />
            </el-form-item>
            <el-form-item label="端口">
              <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" />
            </el-form-item>
          </div>
          <el-form-item label="用户名" prop="username">
            <el-input v-model="form.username" />
          </el-form-item>
          <el-form-item label="认证方式">
            <el-radio-group v-model="form.auth_type">
              <el-radio-button value="password">密码</el-radio-button>
              <el-radio-button value="private_key">私钥</el-radio-button>
            </el-radio-group>
          </el-form-item>
          <el-form-item v-if="form.auth_type === 'password'" :label="form.id ? '密码（不修改请留空）' : '密码'">
            <el-input v-model="form.password" type="password" show-password autocomplete="off" />
          </el-form-item>
          <el-form-item v-else :label="form.id ? '私钥（不修改请留空）' : '私钥'">
            <el-input v-model="form.private_key" type="textarea" :rows="8" class="mono-input" />
          </el-form-item>
          <el-form-item label="状态">
            <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
          </el-form-item>
        </template>

        <template v-else>
          <el-form-item label="地域">
            <el-input v-model="form.region" />
          </el-form-item>
          <el-form-item v-if="form.type === 'docker'" label="Docker 镜像">
            <el-input v-model="form.docker_image" :disabled="!!form.id" />
          </el-form-item>
          <el-alert
            v-if="!form.id"
            :title="form.type === 'docker' ? '创建后直接生成 docker run 命令，请在已安装 Docker 的主机执行。' : '创建后会生成一次性 Token 和部署命令，请及时复制并在目标主机执行。'"
            type="info"
            :closable="false"
            show-icon
          />
        </template>

        <el-form-item label="备注" class="description-field">
          <el-input v-model="form.description" type="textarea" :rows="3" />
        </el-form-item>
        <div class="editor-footer">
          <div><slot name="actions" /></div>
          <div><el-button :disabled="saving" @click="emit('cancel')">取消</el-button><el-button v-permission="['certificate:executor:edit']" type="primary" native-type="submit" :loading="saving">{{ form.type !== 'ssh' && !form.id ? '创建并生成' : '保存' }}</el-button></div>
        </div>
      </el-form>
  </article>
</template>
<script setup>
import { ref } from 'vue'
import { Close } from '@element-plus/icons-vue'
defineProps({ form: { type: Object, required: true }, rules: { type: Object, required: true }, nodeTypes: { type: Array, required: true }, saving: Boolean })
const emit = defineEmits(['save', 'cancel', 'select-type'])
const formRef = ref()
async function submit() { if (await formRef.value.validate().catch(() => false)) emit('save') }
defineExpose({ clearValidate: () => formRef.value?.clearValidate() })
</script>
<style scoped>
.node-editor{margin:0 0 12px;background:var(--ops-surface);border:1px dashed color-mix(in srgb,var(--ops-primary) 45%,var(--ops-border))}
.node-editor.is-editing{margin:-8px 0 8px;border:1px solid var(--ops-primary);border-top:0}
.editor-heading{display:flex;align-items:center;justify-content:space-between;padding:16px 18px;border-bottom:1px solid var(--ops-border-soft)}.editor-heading strong{font-size:15px;color:var(--ops-text)}
.editor-form{padding:20px 22px}.type-card-list{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;width:100%}
.type-card{display:flex;align-items:center;gap:8px;min-height:48px;padding:10px 12px;background:var(--ops-surface);border:1px solid var(--ops-border);color:var(--ops-text);cursor:pointer;font:inherit}
.type-card.active{border-color:var(--ops-primary);background:var(--ops-primary-soft);color:var(--ops-primary)}.type-card.disabled{opacity:.5;cursor:not-allowed}.type-icon{display:flex;font-size:18px}.type-content strong{font-size:12px;font-weight:600}
.form-grid{display:grid;grid-template-columns:minmax(0,1fr) 150px;gap:18px}.form-grid :deep(.el-input-number){width:100%}.description-field{margin-top:18px}
.editor-footer{display:flex;align-items:center;justify-content:space-between;gap:12px;padding-top:16px;border-top:1px solid var(--ops-border-soft)}.editor-footer>div{display:flex;flex-wrap:wrap;gap:8px}.editor-footer :deep(.el-button){margin-left:0}
.mono-input :deep(textarea){font:12px/1.45 ui-monospace,monospace}
@media(max-width:760px){.editor-form{padding:16px}.type-card-list,.form-grid{grid-template-columns:1fr}.editor-footer{align-items:flex-start;flex-direction:column}}
.editor-form :deep(.el-alert--info){background:color-mix(in srgb,var(--ops-primary) 6%,transparent);color:var(--ops-muted)}
.editor-form :deep(.el-alert--info .el-alert__icon){color:var(--ops-primary)}
</style>

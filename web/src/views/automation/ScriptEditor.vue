<template>
  <div class="script-editor-page" v-loading="loading">
    <div class="preview-header">
      <el-button link type="primary" :icon="ArrowLeft" @click="cancel">返回</el-button>
    </div>

    <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="editor-form">
      <section class="form-section metadata-section">
        <div class="section-heading">
          <div class="section-title">
            <strong>基本信息</strong>
          </div>
        </div>
        <div class="metadata-grid">
          <el-form-item class="name-field" label="脚本名称" prop="name">
            <el-input v-model="form.name" maxlength="100" show-word-limit />
          </el-form-item>
          <el-form-item label="运行方式" prop="type">
            <el-select v-model="form.type" aria-label="脚本运行方式"><el-option label="Shell" value="shell" /><el-option label="Python" value="python" /></el-select>
          </el-form-item>
          <el-form-item label="状态">
            <div class="status-field"><el-switch v-model="form.enabled" /><span>{{ form.enabled ? '启用，可被任务调用' : '停用，暂不可调用' }}</span></div>
          </el-form-item>
          <el-form-item class="description-field">
            <template #label>
              <span class="field-label">描述</span>
            </template>
            <el-input v-model="form.description" type="textarea" :rows="2" maxlength="500" show-word-limit />
          </el-form-item>
        </div>
      </section>

      <section class="form-section code-section">
        <div class="section-heading code-heading">
          <div class="section-title">
            <strong>脚本内容</strong>
            <el-tooltip content="支持长脚本编辑、语法高亮与自动换行" placement="top">
              <el-icon class="help-icon" tabindex="0" aria-label="脚本内容说明"><QuestionFilled /></el-icon>
            </el-tooltip>
          </div>
          <span class="language-badge">{{ editorLanguage.toUpperCase() }}</span>
        </div>
        <section class="dashboard-intro" role="note" aria-label="脚本运行时上下文">
          <el-icon class="intro-icon" aria-hidden="true"><InfoFilled /></el-icon>
          <span class="intro-message">标准脚本可按需接收 JSON 输入：手动执行时，仅在脚本需要额外参数时填写；不需要参数时可直接执行。由故障中心触发时，平台会自动传入告警信息。输入内容会原样保存到临时 JSON 文件中，文件路径通过环境变量 AIOPS_INPUT_FILE 传给脚本。Shell 示例：<code>cat ${AIOPS_INPUT_FILE}</code>；Python 示例：<code>path=os.environ["AIOPS_INPUT_FILE"]</code>。<br />当自动化任务选择“证书部署”运行环境时，平台会生成临时证书文件，并向脚本注入以下环境变量：CERT_FILE（证书文件路径）、KEY_FILE（私钥文件路径）、CERT_DEPLOY_DIR（临时工作目录）、DOMAIN（部署域名）和 CERT_NAME（证书名称）。任务执行结束后，平台会自动清理这些临时文件。</span>
        </section>
        <el-form-item prop="content" class="code-field">
          <code-editor v-model="form.content" :language="editorLanguage" height="clamp(480px, calc(100vh - 390px), 760px)" aria-label="脚本内容" />
        </el-form-item>
      </section>


      <editor-action-bar>
        <el-button @click="cancel">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">{{ isEdit ? '保存修改' : '创建脚本' }}</el-button>
      </editor-action-bar>
    </el-form>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, InfoFilled, QuestionFilled } from '@element-plus/icons-vue'
import { createScript, getScript, updateScript } from '../../api/script'
import CodeEditor from '../../components/CodeEditor.vue'
import EditorActionBar from '../../components/EditorActionBar.vue'

const route = useRoute()
const router = useRouter()
const formRef = ref()
const loading = ref(false)
const saving = ref(false)
const form = reactive({ name: '', type: 'shell', content: '', description: '', enabled: true })
const isEdit = computed(() => Boolean(route.params.id))
const editorLanguage = computed(() => form.type === 'python' ? 'python' : 'shell')
const rules = {
  name: [{ required: true, message: '请输入脚本名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择运行方式', trigger: 'change' }],
  content: [{ required: true, message: '请输入脚本内容', trigger: 'blur' }],
}

onMounted(async () => {
  if (!isEdit.value) return
  loading.value = true
  try {
    const script = (await getScript(route.params.id)).data
    Object.assign(form, { name: script.name || '', type: script.type || 'shell', content: script.content || '', description: script.description || '', enabled: script.status === 1 })
  } catch {
    cancel()
  } finally { loading.value = false }
})

function cancel() { router.push('/automation/scripts') }

async function save() {
  if (!await formRef.value.validate().catch(() => false)) return
  saving.value = true
  try {
    const payload = { name: form.name.trim(), type: form.type, source_type: 'inline', content: form.content, description: form.description.trim(), status: form.enabled ? 1 : 0 }
    if (isEdit.value) await updateScript(route.params.id, payload)
    else await createScript(payload)
    ElMessage.success(isEdit.value ? '脚本已更新' : '脚本已创建')
    cancel()
  } finally { saving.value = false }
}
</script>

<style scoped>
.script-editor-page { --ink:var(--ops-text); --muted:var(--ops-muted); --border:var(--ops-border); display:flex; min-height:calc(100vh - 100px); flex-direction:column; gap:14px; padding-bottom:72px; }
.preview-header { display:flex; align-items:center; justify-content:space-between; gap:20px; }
.editor-form { display:flex; min-width:0; flex:1; flex-direction:column; gap:14px; }
.form-section { padding:20px 22px; background:var(--ops-surface); border:1px solid var(--border); border-radius:10px; box-shadow:0 5px 18px rgba(31,50,81,.035); }
.section-heading { display:flex; align-items:center; justify-content:space-between; gap:14px; margin-bottom:18px; }
.section-title,.field-label { display:inline-flex; align-items:center; gap:5px; }
.section-heading strong { color:var(--ink); font-size:15px; }
.help-icon { color:#9aa6b7; cursor:help; font-size:14px; outline:none; }
.help-icon:hover,.help-icon:focus { color:var(--el-color-primary); }
.metadata-grid { display:grid; grid-template-columns:minmax(260px,2fr) minmax(150px,1fr) minmax(220px,1fr); gap:0 18px; }
.description-field { grid-column:1/-1; }
.status-field { display:flex; min-height:32px; align-items:center; gap:9px; color:#69778a; font-size:12px; }
.code-section { min-width:0; padding-bottom:22px; }
.code-heading { margin-bottom:12px; }
.code-heading .language-badge { padding:5px 9px; color:#3c6aa1; background:#edf5ff; border-radius:5px; font-family:ui-monospace,SFMono-Regular,Menlo,monospace; font-size:10px; font-weight:700; letter-spacing:.4px; }
.dashboard-intro { display:flex; min-height:36px; align-items:center; gap:9px; margin-bottom:12px; padding:7px 12px; color:var(--ops-muted); background:var(--ops-primary-soft); font-size:11px; }
.intro-icon { flex:none; color:var(--ops-primary); font-size:14px; }
.intro-message { min-width:0; color:var(--ops-muted); line-height:1.7; }
.intro-message code { color:#315f95; font:10px ui-monospace,SFMono-Regular,Menlo,monospace; }
.code-field { margin-bottom:0; }
:deep(.el-select) { width:100%; }
@media (max-width:900px) { .metadata-grid { grid-template-columns:1fr 1fr; }.name-field,.description-field { grid-column:1/-1; } }
@media (max-width:700px) { .metadata-grid { grid-template-columns:1fr; }.name-field,.description-field { grid-column:auto; }.form-section { padding:18px; } }
</style>

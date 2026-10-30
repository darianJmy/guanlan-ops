<template>
  <div class="page-container">
    <div class="fault-toolbar">
      <div class="fault-filters" role="search" aria-label="故障中心筛选">
        <el-input v-model="keyword" class="keyword-filter" :prefix-icon="Search" aria-label="搜索故障中心名称" placeholder="搜索故障中心名称" clearable />
      </div>
      <div class="toolbar-actions">
        <el-button :icon="Refresh" :loading="loading" @click="loadPage">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="openCenterDialog()">创建</el-button>
      </div>
    </div>

    <div v-if="loading || filteredCenters.length > 0" v-loading="loading" class="dashboard-grid">
      <article
        v-for="center in filteredCenters"
        :key="center.id"
        class="dash-card"
        @click="openAlerts(center)"
      >
        <div class="card-head">
          <h2>{{ center.name }}</h2>
        </div>
        <p v-if="center.description?.trim()" class="card-description">{{ center.description }}</p>
        <div class="card-metrics">
          <div class="metric-pending"><strong>{{ center.pending_count || 0 }}</strong><span>预告警</span></div>
          <div class="metric-firing"><strong>{{ center.firing_count || 0 }}</strong><span>告警中</span></div>
          <div class="metric-recovering"><strong>{{ center.recovering_count || 0 }}</strong><span>恢复中</span></div>
          <div class="metric-silenced"><strong>{{ center.silenced_count || 0 }}</strong><span>通知静默</span></div>
        </div>
        <div class="card-footer" @click.stop>
          <div class="card-actions">
            <el-tooltip content="编辑" placement="top">
              <el-button link type="primary" size="small" aria-label="编辑" @click="openCenterDialog(center)">
                <el-icon><Edit /></el-icon>
              </el-button>
            </el-tooltip>
            <el-tooltip :content="center.is_default ? '默认故障中心不能删除' : '删除'" placement="top">
              <el-button link type="danger" size="small" aria-label="删除" :disabled="center.is_default" @click="removeCenter(center)">
                <el-icon><Delete /></el-icon>
              </el-button>
            </el-tooltip>
          </div>
        </div>
      </article>
    </div>

    <PageEmpty v-if="!loading && filteredCenters.length === 0" :description="centers.length ? '没有符合条件的故障中心' : '暂无故障中心'" />

    <el-dialog v-model="centerDialogVisible" :title="centerForm.id ? '编辑故障中心' : '创建故障中心'" width="min(460px, 94vw)" :close-on-click-modal="false" destroy-on-close>
      <el-form ref="centerFormRef" class="center-form" :model="centerForm" :rules="centerRules" label-position="top">
        <el-form-item label="中心名称" prop="name">
          <el-input v-model="centerForm.name" aria-label="故障中心名称" maxlength="128" show-word-limit />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="centerForm.description" aria-label="故障中心描述" type="textarea" :rows="4" maxlength="500" show-word-limit />
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="centerDialogVisible = false">取消</el-button>
          <el-button type="primary" :loading="savingCenter" @click="saveCenter">保存</el-button>
        </div>
      </template>
    </el-dialog>

  </div>
</template>

<script setup>
import PageEmpty from '../../components/PageEmpty.vue'
import { onMounted, reactive, ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete, Edit, Plus, Refresh, Search } from '@element-plus/icons-vue'
import { createFaultCenter, deleteFaultCenter, getFaultCenters, updateFaultCenter } from '../../api/alert'

const router = useRouter()
const loading = ref(false), savingCenter = ref(false)
const centers = ref([])
const centerDialogVisible = ref(false)
const centerFormRef = ref()
const keyword = ref('')
const centerForm = reactive({ id: null, name: '', description: '', status: 1 })
const centerRules = { name: [{ required: true, message: '请输入故障中心名称', trigger: 'blur' }] }
const filteredCenters = computed(() => {
  const query = keyword.value.trim().toLowerCase()
  return centers.value.filter(center => (center.name || '').toLowerCase().includes(query))
})

onMounted(loadPage)

async function loadPage() {
  loading.value = true
  try {
    const centerRes = await getFaultCenters()
    centers.value = centerRes.data || []
  } finally {
    loading.value = false
  }
}

function openCenterDialog(center) {
  Object.assign(centerForm, center
    ? { id: center.id, name: center.name, description: center.description || '', status: center.status }
    : { id: null, name: '', description: '', status: 1 })
  centerDialogVisible.value = true
}

async function saveCenter() {
  const valid = await centerFormRef.value?.validate().catch(() => false)
  if (!valid) return
  savingCenter.value = true
  try {
    const payload = { name: centerForm.name.trim(), description: centerForm.description.trim(), status: centerForm.status }
    if (centerForm.id) await updateFaultCenter(centerForm.id, payload)
    else await createFaultCenter(payload)
    ElMessage.success(centerForm.id ? '故障中心已更新' : '故障中心已创建')
    centerDialogVisible.value = false
    await loadPage()
  } finally {
    savingCenter.value = false
  }
}

async function removeCenter(center) {
  if (center.is_default) return
  await ElMessageBox.confirm('已有拨测任务或告警的中心不能删除。', '删除故障中心', { type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消' })
  await deleteFaultCenter(center.id)
  ElMessage.success('故障中心已删除')
  await loadPage()
}

function openAlerts(center) {
  router.push(`/alerts/${center.id}`)
}
</script>

<style scoped>
.page-container { display: flex; flex-direction: column; gap: 16px; }
.fault-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.fault-filters, .toolbar-actions { display: flex; align-items: center; gap: 8px; }
.fault-filters { min-width: 0; flex: 1; }
.keyword-filter { width: 200px; }
.toolbar-actions { flex: 0 0 auto; }
.dashboard-grid { display: grid; min-height: 150px; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.dash-card { display: flex; min-width: 0; min-height: 178px; overflow: hidden; flex-direction: column; background: var(--ops-surface); border: 1px solid var(--ops-border-soft); border-radius: 10px; cursor: pointer; transition: border-color .2s ease, box-shadow .2s ease, transform .2s ease; }
.dash-card:hover { border-color: var(--ops-primary); box-shadow: 0 4px 16px rgba(64, 158, 255, .12); transform: translateY(-2px); }
.dash-card:focus-within { outline: 2px solid rgba(64, 158, 255, .28); outline-offset: 2px; }
.card-head { display: flex; align-items: center; gap: 12px; padding: 20px 20px 0; }
.card-head h2 { min-width: 0; margin: 0; overflow: hidden; color: var(--ops-text); font-size: 15px; font-weight: 600; letter-spacing: 0; text-overflow: ellipsis; white-space: nowrap; }
.card-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 8px; }
.card-actions .el-button + .el-button { margin-left: 0; }
.card-description { display: -webkit-box; margin: 4px 0 0; padding: 0 20px 14px; overflow: hidden; color: var(--ops-muted); font-size: 12px; line-height: 18px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.card-metrics { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: auto; padding: 10px 14px; gap: 4px 20px; background: var(--ops-surface-soft); border-top: 1px solid #f5f5f5; }
.card-metrics div { display: flex; min-width: 0; align-items: center; justify-content: flex-start; gap: 8px; padding: 4px 0; }
.card-metrics strong { color: #475467; font-size: 18px; font-weight: 600; line-height: 22px; }
.card-metrics span { color: var(--ops-muted); font-size: 11px; white-space: nowrap; }
.metric-pending strong { color: #d98b22; }
.metric-firing strong { color: #d33f50; }
.metric-recovering strong { color: #2f9e6f; }
.metric-silenced strong { color: #7b8492; }
.card-footer { display: flex; align-items: center; justify-content: flex-end; padding: 10px 20px; background: var(--ops-surface-soft); border-top: 1px solid #f5f5f5; }
.dialog-footer { display:flex;justify-content:flex-end;gap:8px; }
@media (prefers-reduced-motion: reduce) { .dash-card { transition: none; } }
@media (max-width: 900px) { .fault-toolbar { align-items: stretch; flex-direction: column; } .keyword-filter { width: 100%; } .toolbar-actions { justify-content: flex-end; } }
@media (max-width: 640px) { .fault-filters { flex-wrap: wrap; } .keyword-filter { width: 100%; flex: 1 1 100%; } .toolbar-actions { display: grid; grid-template-columns: 1fr 1fr; } .dashboard-grid { grid-template-columns: 1fr; } .dash-card { min-height: 184px; } }
</style>

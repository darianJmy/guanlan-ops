<template>
  <section>
    <el-radio-group
      v-if="canViewDashboards && canViewServices"
      class="dashboard-sections"
      :model-value="activeSection"
      aria-label="仪表盘类型"
      @change="switchSection"
    >
      <el-radio-button value="dashboards">监控与日志</el-radio-button>
      <el-radio-button value="services">服务仪表盘</el-radio-button>
    </el-radio-group>

    <ServiceDashboardManage v-if="activeSection === 'services'" />
    <DashboardHub v-else-if="activeSection === 'dashboards'" />
    <PageEmpty v-else description="没有仪表盘查看权限" />
  </section>
</template>

<script setup>
import { computed, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useUserStore } from '../../stores/user'
import PageEmpty from '../../components/PageEmpty.vue'

const DashboardHub = defineAsyncComponent(() => import('./DashboardHub.vue'))
const ServiceDashboardManage = defineAsyncComponent(() => import('./ServiceDashboardManage.vue'))
const store = useUserStore()
const route = useRoute()
const router = useRouter()

const canViewDashboards = computed(() => store.hasPermission('dashboard:list'))
const canViewServices = computed(() => store.hasPermission('service-dashboard:list'))
const activeSection = computed(() => {
  if (canViewServices.value && (!canViewDashboards.value || route.query.tab === 'services')) return 'services'
  if (canViewDashboards.value) return 'dashboards'
  return ''
})

function switchSection(section) {
  const query = { ...route.query, tab: section === 'services' ? 'services' : undefined, folder: undefined }
  router.push({ path: route.path, query })
}
</script>

<style scoped>
.dashboard-sections {
  margin-bottom: 16px;
}
</style>

import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '../stores/user'
import { getToken } from '../utils/auth'

const staticRoutes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue'),
    meta: { title: '登录', noAuth: true },
  },
  {
    path: '/',
    name: 'Layout',
    component: () => import('../views/Layout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'system/permissions', redirect: '/dashboard' },
      { path: 'system/menus', redirect: '/dashboard' },
      { path: 'settings/api-tokens', redirect: '/dashboard' },
      { path: 'explore', redirect: '/monitor/query' },
      { path: 'observability/logs', redirect: '/monitor/query?kind=logs' },
      { path: 'observability/traces', redirect: '/monitor/query?kind=traces' },
      { path: 'observability', redirect: '/monitor/query' },
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../views/Dashboard.vue'),
        meta: { title: '仪表盘' },
      },
      {
        path: 'inspection/items/groups/:groupId',
        name: 'InspectionRuleGroupTasks',
        component: () => import('../views/inspection/InspectionManage.vue'),
        meta: { title: '拨测规则组', activeMenu: '/inspection/items', breadcrumbParent: { title: '拨测任务', path: '/inspection/items' } },
      },
      {
        path: 'inspection/items/create',
        name: 'InspectionTaskCreate',
        component: () => import('../views/inspection/InspectionTaskEditor.vue'),
        meta: { title: '创建拨测任务', activeMenu: '/inspection/items' },
      },
      {
        path: 'inspection/items/:id',
        name: 'InspectionTaskDetail',
        component: () => import('../views/inspection/InspectionTaskDetail.vue'),
        meta: { title: '拨测任务详情', activeMenu: '/inspection/items' },
      },
      {
        path: 'inspection/items/:id/edit',
        name: 'InspectionTaskEdit',
        component: () => import('../views/inspection/InspectionTaskEditor.vue'),
        meta: { title: '编辑拨测任务', activeMenu: '/inspection/items' },
      },
      {
        path: 'automation/scripts/create',
        name: 'ScriptCreate',
        component: () => import('../views/automation/ScriptEditor.vue'),
        meta: { title: '创建脚本', activeMenu: '/automation/scripts' },
      },
      {
        path: 'automation/scripts/:id/edit',
        name: 'ScriptEdit',
        component: () => import('../views/automation/ScriptEditor.vue'),
        meta: { title: '编辑脚本', activeMenu: '/automation/scripts' },
      },
      {
        path: 'automation/workflows/create',
        name: 'WorkflowCreate',
        component: () => import('../views/inspection/WorkflowManage.vue'),
        meta: { title: '创建工作流', activeMenu: '/automation/workflows' },
      },
      {
        path: 'automation/workflows/:id/edit',
        name: 'WorkflowEdit',
        component: () => import('../views/inspection/WorkflowManage.vue'),
        meta: { title: '编辑工作流', activeMenu: '/automation/workflows' },
      },
      {
        path: 'alerts/:id',
        name: 'FaultCenterDetail',
        component: () => import('../views/alert/FaultCenterDetail.vue'),
        meta: { title: '故障中心详情', activeMenu: '/alerts', breadcrumbParent: { title: '故障中心', path: '/alerts' } },
      },
      {
        path: 'alert-rules/:groupId/create',
        name: 'AlertRuleCreate',
        component: () => import('../views/alert/RuleEditor.vue'),
        meta: { title: '创建告警规则', activeMenu: '/alert-rules' },
      },
      {
        path: 'alert-rules/:groupId/:ruleId/edit',
        name: 'AlertRuleEdit',
        component: () => import('../views/alert/RuleEditor.vue'),
        meta: { title: '编辑告警规则', activeMenu: '/alert-rules' },
      },
      {
        path: 'alert-rules/:groupId',
        name: 'AlertRuleGroupDetail',
        component: () => import('../views/alert/RuleManage.vue'),
        meta: { title: '规则组详情', activeMenu: '/alert-rules', breadcrumbParent: { title: '告警管理', path: '/alert-rules' } },
      },
      {
        path: ':pathMatch(.*)*',
        name: 'NotFound',
        component: () => import('../views/common/NotFound.vue'),
        meta: { title: '页面不存在' },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes: staticRoutes,
})

// 组件路径映射表，用于动态路由 addRoute
const componentMap = {
  'system/UserManage': () => import('../views/system/UserManage.vue'),
  'system/RoleManage': () => import('../views/system/RoleManage.vue'),
  'system/GitCredentialManage': () => import('../views/system/GitCredentialManage.vue'),
  'system/BasicSetting': () => import('../views/system/BasicSetting.vue'),
  'system/AIModelManage': () => import('../views/system/AIModelManage.vue'),
  'system/AuthSetting': () => import('../views/system/AuthSetting.vue'),
  'monitor/IntegrationManage': () => import('../views/monitor/IntegrationManage.vue'),
  'monitor/DashboardManage': () => import('../views/monitor/DashboardManage.vue'),
  'monitor/DashboardHub': () => import('../views/monitor/DashboardHub.vue'),
  'monitor/GrafanaDashboardManage': () => import('../views/monitor/GrafanaDashboardManage.vue'),
  'monitor/SLSDashboardManage': () => import('../views/monitor/SLSDashboardManage.vue'),
  'monitor/CloudMonitorDashboardManage': () => import('../views/monitor/CloudMonitorDashboardManage.vue'),
  'monitor/LogQuery': () => import('../views/monitor/LogQuery.vue'),
  'monitor/Explore': () => import('../views/monitor/Explore.vue'),
  'monitor/InstantQuery': () => import('../views/monitor/InstantQuery.vue'),
  'common/FeaturePlaceholder': () => import('../views/common/FeaturePlaceholder.vue'),
  'notification/ChannelManage': () => import('../views/notification/ChannelManage.vue'),
  'notification/TemplateManage': () => import('../views/notification/TemplateManage.vue'),
  'notification/RecordManage': () => import('../views/notification/RecordManage.vue'),
  'security/AuditLog': () => import('../views/security/AuditLog.vue'),
  'alert/RuleGroupList': () => import('../views/alert/RuleGroupList.vue'),
  'alert/AlertCenter': () => import('../views/alert/AlertCenter.vue'),
  'certificate/ExecutorManage': () => import('../views/certificate/ExecutorManage.vue'),
  'automation/ScriptLibrary': () => import('../views/automation/ScriptLibrary.vue'),
	'automation/AutomationTaskManage': () => import('../views/automation/AutomationTaskManage.vue'),
  'inspection/InspectionRuleGroupList': () => import('../views/inspection/InspectionRuleGroupList.vue'),
  'inspection/WorkflowManage': () => import('../views/inspection/WorkflowManage.vue'),
  'inspection/ExecutionRecords': () => import('../views/inspection/ExecutionRecords.vue'),
}

const dynamicRouteRemovers = []

function resetDynamicRoutes() {
  dynamicRouteRemovers.splice(0).forEach(removeRoute => removeRoute())
}

function addDynamicRoutes(menus) {
  const addRouteFromMenu = (items, parentPath = '') => {
    items.forEach((menu) => {
      if (menu.type === 2 && componentMap[menu.component]) {
        const routePath = menu.path.replace(/^\//, '')
        const removeRoute = router.addRoute('Layout', {
          path: routePath,
          name: menu.name,
          component: componentMap[menu.component],
          meta: { title: menu.name },
        })
        dynamicRouteRemovers.push(removeRoute)
      }
      if (menu.children && menu.children.length > 0) {
        addRouteFromMenu(menu.children, parentPath)
      }
    })
  }
  addRouteFromMenu(menus)
}

router.beforeEach(async (to, from, next) => {
  document.title = to.meta.title ? `${to.meta.title} - 观澜运维平台` : '观澜运维平台'

  const store = useUserStore()
  // Profile and routes belong to the current login, not the page lifetime.
  if (!store.userInfo) resetDynamicRoutes()

  if (to.meta.noAuth) {
    next()
    return
  }

  const token = getToken()
  if (!token) {
    store.logout()
    resetDynamicRoutes()
    next('/login')
    return
  }

  if (!store.userInfo) {
    try {
      await store.fetchUserInfo()
      addDynamicRoutes(store.menus)
      next({ path: to.path, query: to.query, hash: to.hash, replace: true })
    } catch {
      store.logout()
      resetDynamicRoutes()
      next('/login')
    }
    return
  }

  next()
})

export default router

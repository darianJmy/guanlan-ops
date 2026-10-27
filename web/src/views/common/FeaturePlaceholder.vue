<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <span class="page-title">{{ feature.title }}</span>
        <p class="page-subtitle">{{ feature.summary }}</p>
      </div>
      <el-tag :type="feature.statusType">{{ feature.status }}</el-tag>
    </div>

    <section class="overview-band">
      <div class="section-title">模块边界</div>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="阶段">{{ feature.phase }}</el-descriptions-item>
        <el-descriptions-item label="优先级">{{ feature.priority }}</el-descriptions-item>
        <el-descriptions-item label="主要数据">{{ feature.data }}</el-descriptions-item>
        <el-descriptions-item label="关键风险">{{ feature.risk }}</el-descriptions-item>
      </el-descriptions>
    </section>

    <section class="overview-band">
      <div class="section-title">能力规划</div>
      <div class="feature-grid">
        <div v-for="item in feature.items" :key="item" class="feature-item">
          <el-icon><Check /></el-icon>
          <span>{{ item }}</span>
        </div>
      </div>
    </section>

    <section class="overview-band">
      <div class="section-title">近期落地任务</div>
      <el-table :data="feature.tasks" size="small" border>
        <el-table-column prop="name" label="任务" min-width="180" />
        <el-table-column prop="deliverable" label="交付物" min-width="220" />
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.status === '已完成' ? 'success' : 'info'" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <template #empty><TableEmpty /></template>
      </el-table>
    </section>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { Check } from '@element-plus/icons-vue'

const route = useRoute()

const features = {
  '/observability/logs': {
    title: '日志查询',
    summary: '接入 Loki、Elasticsearch、SLS、CLS 等日志源，提供统一日志检索和故障上下文分析。',
    status: '规划中',
    statusType: 'info',
    phase: '第二阶段：可观测能力增强',
    priority: 'P1',
    data: '日志数据源、查询历史、检索模板',
    risk: '大查询超时、日志权限隔离、敏感内容脱敏',
    items: ['LogQL 查询', 'Elasticsearch DSL 查询', '日志上下文跳转', '日志摘要', '查询收藏', '结果导出'],
    tasks: [
      { name: '日志数据源协议', deliverable: '统一日志查询请求和响应结构', status: '待实现' },
      { name: '日志查询页面', deliverable: '时间范围、关键词、标签过滤、结果列表', status: '待实现' },
      { name: '日志脱敏', deliverable: '手机号、Token、密钥等字段脱敏', status: '待实现' },
    ],
  },
  '/observability/traces': {
    title: '链路追踪',
    summary: '接入 Jaeger、OpenTelemetry 等链路系统，用于慢调用、错误链路和上下游依赖分析。',
    status: '规划中',
    statusType: 'info',
    phase: '第二阶段：可观测能力增强',
    priority: 'P1',
    data: 'Trace、Span、服务依赖',
    risk: 'Trace 体量大、跨系统 TraceID 关联不完整',
    items: ['Trace 查询', '慢调用分析', '错误 Span 定位', '服务依赖图', '告警关联 Trace', 'AI 解释调用链'],
    tasks: [
      { name: 'Trace 数据源', deliverable: 'Jaeger 查询接口封装', status: '待实现' },
      { name: 'Trace 详情页', deliverable: 'Span 时间线和错误节点展示', status: '待实现' },
      { name: '依赖分析', deliverable: '从 Trace 聚合服务调用关系', status: '待实现' },
    ],
  },
  '/alerts/events': {
    title: '告警事件',
    summary: '接入 Alertmanager、Grafana Alerting、云监控和巡检告警，形成统一告警处理入口。',
    status: '规划中',
    statusType: 'warning',
    phase: '第四阶段：告警治理',
    priority: 'P0',
    data: '告警事件、告警状态、处理记录',
    risk: '告警风暴、重复告警、通知噪音',
    items: ['告警接入', '告警列表', '告警认领', '告警关闭', '告警聚合', '告警静默'],
    tasks: [
      { name: 'Alertmanager Webhook', deliverable: '接收和落库 Prometheus 告警', status: '待实现' },
      { name: '告警列表', deliverable: '按状态、级别、服务过滤告警', status: '待实现' },
      { name: '告警处理流转', deliverable: '认领、关闭、备注、审计', status: '待实现' },
    ],
  },
  '/notifications/channels': {
    title: '通知对象',
    summary: '统一管理飞书、钉钉、企业微信、邮件和 Webhook，用于告警、巡检、证书和审批通知。',
    status: '规划中',
    statusType: 'info',
    phase: '第四至第五阶段：告警治理和工作流',
    priority: 'P1',
    data: '通知对象、通知模板、发送记录',
    risk: '通知风暴、密钥泄露、失败重试',
    items: ['飞书', '钉钉', '企业微信', '邮件', 'Webhook', '通知升级'],
    tasks: [
      { name: '对象模型', deliverable: '通知对象配置和密钥脱敏', status: '待实现' },
      { name: '发送接口', deliverable: '统一通知发送 API', status: '待实现' },
      { name: '通知记录', deliverable: '发送结果、失败原因、重试次数', status: '待实现' },
    ],
  },
  '/security/audit': {
    title: '操作审计',
    summary: '记录登录、写操作、外部 API 调用和高危操作，为平台安全和问题追踪提供依据。',
    status: '规划中',
    statusType: 'danger',
    phase: '基础平台长期能力',
    priority: 'P0',
    data: '用户、动作、资源、请求摘要、结果',
    risk: '审计缺失会导致高危操作不可追溯',
    items: ['登录审计', '操作审计', 'API 调用审计', '敏感字段脱敏', '审计导出', '高危动作审批'],
    tasks: [
      { name: '审计中间件', deliverable: '统一记录写操作和操作者', status: '待实现' },
      { name: '审计列表', deliverable: '按用户、资源、动作、时间过滤', status: '待实现' },
      { name: '敏感字段规则', deliverable: 'Token、AK、私钥统一脱敏', status: '待实现' },
    ],
  },
}

const fallback = {
  title: route.meta.title || '功能规划',
  summary: '该模块已在路线图中预留入口，后续会按 README 的实施阶段逐步落地。',
  status: '规划中',
  statusType: 'info',
  phase: '待排期',
  priority: 'P2',
  data: '待定义',
  risk: '需求边界待细化',
  items: ['数据模型', 'API 接口', '管理页面', '权限控制', '执行记录', '审计追踪'],
  tasks: [
    { name: '需求拆解', deliverable: '明确模块边界和非目标', status: '待实现' },
    { name: '数据建模', deliverable: '新增模型、迁移和种子数据', status: '待实现' },
    { name: '页面实现', deliverable: '列表、详情、创建和编辑流程', status: '待实现' },
  ],
}

const feature = computed(() => features[route.path] || fallback)
</script>

<style scoped>
.page-container { display: flex; flex-direction: column; gap: 12px; }
.page-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; }
.page-title { font-size: 18px; font-weight: 700; color: var(--ops-text); }
.page-subtitle { margin: 8px 0 0; max-width: 760px; color: var(--ops-muted); line-height: 1.6; }
.overview-band {
  background: var(--ops-surface);
  border: 1px solid var(--ops-border);
  border-radius: 8px;
  padding: 16px;
}
.section-title { font-size: 14px; font-weight: 700; color: var(--ops-text); margin-bottom: 12px; }
.feature-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 10px;
}
.feature-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 8px 10px;
  background: #f7f9fc;
  border: 1px solid var(--ops-border-soft);
  border-radius: 6px;
  color: var(--ops-text);
  font-size: 13px;
}
.feature-item .el-icon { color: #67c23a; }

@media (max-width: 760px) {
  .page-header { flex-direction: column; }
  .feature-grid { grid-template-columns: 1fr; }
}
</style>

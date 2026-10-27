<template>
  <VueFlow
    class="workflow-flow"
    :nodes="nodes"
    :edges="edges"
    :min-zoom="0.35"
    :max-zoom="1.5"
    :nodes-draggable="true"
    :nodes-connectable="true"
    :nodes-deletable="false"
    :edges-focusable="true"
    :edges-deletable="true"
    :fit-view-on-init="true"
    :fit-view-on-init-options="{ padding: 0.2, maxZoom: 1 }"
    @init="handleInit"
    @node-click="handleNodeClick"
    @node-drag-stop="handleNodeDragStop"
    @connect="handleConnect"
    @edges-delete="handleEdgesDelete"
  >
    <template #node-terminal="{ data }">
      <div class="flow-terminal" :class="data.kind">
        <el-icon><VideoPlay /></el-icon>
        <strong>{{ data.label }}</strong>
        <Handle type="source" :position="Position.Bottom" />
      </div>
    </template>

    <template #node-operation="{ data }">
      <article class="flow-operation" :class="[`node-${data.step.type}`, { selected: data.selected }]">
        <Handle type="target" :position="Position.Top" />
        <span class="node-index">{{ data.order }}</span>
        <div class="node-copy"><small>{{ data.meta.short }}</small><strong>{{ data.step.name || data.meta.label }}</strong><span>{{ data.summary }}</span></div>
        <el-icon v-if="!['output', 'end'].includes(data.step.type)" class="node-arrow"><Right /></el-icon>
        <Handle v-if="!['output', 'end'].includes(data.step.type)" id="next" type="source" :position="Position.Bottom" />
      </article>
    </template>

    <template #node-condition="{ data }">
      <div class="flow-condition" :class="{ selected: data.selected }">
        <Handle type="target" :position="Position.Top" />
        <div class="condition-diamond"><div><small>IF</small><strong>{{ data.step.name || '条件判断' }}</strong><span>{{ data.summary }}</span></div></div>
        <span class="condition-branch-label branch-success">成功</span>
        <Handle id="true" type="source" :position="Position.Left" />
        <span class="condition-branch-label branch-failure">失败</span>
        <Handle id="false" type="source" :position="Position.Right" />
      </div>
    </template>
  </VueFlow>
</template>

<script setup>
import { computed, nextTick, watch } from 'vue'
import { Right, VideoPlay } from '@element-plus/icons-vue'
import { Handle, MarkerType, Position, VueFlow } from '@vue-flow/core'
import '@vue-flow/core/dist/style.css'
import '@vue-flow/core/dist/theme-default.css'

const props = defineProps({
  steps: { type: Array, default: () => [] },
  selectedIndex: { type: Number, default: -1 },
  stepMeta: { type: Function, required: true },
  summaries: { type: Array, default: () => [] },
})
const emit = defineEmits(['select', 'connect', 'disconnect', 'move'])
let flowInstance

const stepNodeID = index => `step-${index}`
const nodeY = index => 135 + index * 150
const nodes = computed(() => [
  { id: 'start', type: 'terminal', position: { x: 105, y: 20 }, data: { kind: 'start', label: '开始' }, selectable: false, draggable: false },
  ...props.steps.map((step, index) => ({
    id: stepNodeID(index), type: step.type === 'condition' ? 'condition' : 'operation',
    position: step.position || { x: step.type === 'condition' ? 55 : 0, y: nodeY(index) },
    data: { index, order: String(index + 1).padStart(2, '0'), step, meta: props.stepMeta(step.type), summary: props.summaries[index] || '', selected: props.selectedIndex === index },
  })),
])

const edgeBase = { type: 'smoothstep', markerEnd: MarkerType.ArrowClosed, style: { stroke: '#8ca6c2', strokeWidth: 1.6 } }
function branchTargetNode(target) {
  if (!target) return null
  const targetIndex = props.steps.findIndex(step => step.key === target)
  return targetIndex < 0 ? null : stepNodeID(targetIndex)
}
const edges = computed(() => {
  const result = []
  props.steps.forEach((step, index) => {
    if (step.start) result.push({ ...edgeBase, id: `edge-start-${index}`, source: 'start', target: stepNodeID(index), data: { kind: 'start', targetIndex: index } })
  })
  props.steps.forEach((step, index) => {
    const source = stepNodeID(index)
    if (step.type !== 'condition') {
	  if (step.type === 'end' || step.type === 'output') return
      ;(step.next || []).forEach((target, targetOrder) => {
        const targetNode = branchTargetNode(target)
        if (targetNode) result.push({ ...edgeBase, id: `edge-${index}-next-${targetOrder}`, source, sourceHandle: 'next', target: targetNode, data: { kind: 'next', sourceIndex: index, target } })
      })
      return
    }
    const trueTarget = branchTargetNode(step.true_next)
    const falseTarget = branchTargetNode(step.false_next)
    if (trueTarget) result.push({ ...edgeBase, id: `edge-${index}-true`, source, sourceHandle: 'true', target: trueTarget, label: '成功', data: { kind: 'true', sourceIndex: index }, labelStyle: { fill: '#188865', fontSize: 10, fontWeight: 700 }, style: { stroke: '#53a98d', strokeWidth: 1.7 } })
    if (falseTarget) result.push({ ...edgeBase, id: `edge-${index}-false`, source, sourceHandle: 'false', target: falseTarget, label: '失败', data: { kind: 'false', sourceIndex: index }, labelStyle: { fill: '#b15a55', fontSize: 10, fontWeight: 700 }, style: { stroke: '#c48b86', strokeWidth: 1.7 } })
  })
  return result
})

function handleInit(instance) { flowInstance = instance }
function handleNodeClick({ node }) { if (Number.isInteger(node.data?.index)) emit('select', node.data.index) }
function handleNodeDragStop({ node }) { if (Number.isInteger(node.data?.index)) emit('move', { index: node.data.index, position: node.position }) }
function handleConnect(connection) { emit('connect', connection) }
function handleEdgesDelete(deletedEdges) { deletedEdges.forEach(edge => emit('disconnect', edge.data)) }
watch(() => props.steps.length, async () => { await nextTick(); flowInstance?.fitView({ padding: 0.2, maxZoom: 1, duration: 180 }) })
</script>

<style>
.workflow-flow{width:100%;height:100%;background:transparent}.workflow-flow .vue-flow__pane{cursor:grab}.workflow-flow .vue-flow__node{border:0;background:transparent;box-shadow:none}.workflow-flow .vue-flow__handle{width:11px;height:11px;background:var(--ops-surface);border:2px solid #4f82bb;cursor:crosshair}.workflow-flow .vue-flow__edge.selected path{stroke:#1769df!important;stroke-width:2.5!important}.workflow-flow .vue-flow__edge-label{padding:2px 5px;background:var(--ops-surface);border-radius:8px}.flow-terminal{position:relative;display:flex;width:122px;height:44px;align-items:center;justify-content:center;gap:7px;color:#405269;background:var(--ops-surface);border:1px solid #cbd7e3;border-radius:24px;box-shadow:0 5px 14px rgba(38,55,79,.09);font-size:11px}.flow-terminal.start .el-icon{color:#16906b}.flow-operation{position:relative;display:flex;width:330px;min-height:86px;align-items:center;gap:13px;padding:14px 16px;color:#2e4058;background:var(--ops-surface);border:1px solid #d4dee9;border-left:4px solid #3478cc;border-radius:10px;box-shadow:0 7px 20px rgba(36,57,85,.1);transition:.15s}.flow-operation.selected{border-color:#4388df;border-left-color:#1769df;box-shadow:0 0 0 3px rgba(44,123,222,.15),0 10px 24px rgba(36,71,115,.12)}.flow-operation.node-input{border-left-color:#209b6c}.flow-operation.node-output,.flow-operation.node-end{border-left-color:#5068b8}.flow-operation.node-http{border-left-color:#8b62bd}.flow-operation.node-inspection{border-left-color:#178f81}.flow-operation.node-delay{border-left-color:#bf8124}.flow-operation.node-notification{border-left-color:#c85e69}.flow-operation .node-index{display:flex;width:38px;height:38px;flex:0 0 38px;align-items:center;justify-content:center;color:#3973ba;background:#edf4fd;border-radius:9px;font-size:10px;font-weight:800}.flow-operation .node-copy{display:flex;min-width:0;flex:1;flex-direction:column}.flow-operation .node-copy small{color:#8996a7;font-size:9px;font-weight:700;letter-spacing:.4px}.flow-operation .node-copy strong{margin-top:4px;color:#28394f;font-size:12px}.flow-operation .node-copy span{max-width:220px;margin-top:4px;overflow:hidden;color:#8290a2;font-size:9px;text-overflow:ellipsis;white-space:nowrap}.flow-operation .node-arrow{color:#9aa8b7}.flow-condition{position:relative;width:240px;height:122px}.flow-condition .condition-diamond{position:absolute;top:17px;left:78px;width:84px;height:84px;background:#fffaf0;border:1px solid #dfbd74;border-radius:12px;box-shadow:0 7px 18px rgba(126,88,24,.12);transform:rotate(45deg);transition:.15s}.flow-condition.selected .condition-diamond{border-color:#d59a28;box-shadow:0 0 0 3px rgba(213,154,40,.16),0 8px 20px rgba(126,88,24,.14)}.flow-condition .condition-diamond>div{display:flex;width:84px;height:84px;align-items:center;justify-content:center;flex-direction:column;transform:rotate(-45deg)}.flow-condition small{color:#b07b1c;font-size:9px;font-weight:800}.flow-condition strong{max-width:92px;margin-top:3px;overflow:hidden;color:#624b20;font-size:11px;text-overflow:ellipsis;white-space:nowrap}.flow-condition .condition-diamond span{max-width:100px;margin-top:3px;overflow:hidden;color:#9a7a3e;font-size:8px;text-overflow:ellipsis;white-space:nowrap}.flow-condition .condition-branch-label{position:absolute;top:51px;margin:0;font-size:9px;font-weight:750;line-height:16px}.flow-condition .branch-success{left:5px;color:#188865}.flow-condition .branch-failure{right:5px;color:#b15a55}.flow-condition .vue-flow__handle-top{top:6px}.flow-condition .vue-flow__handle-left{left:58px;top:50%;background:#eaf9f3;border-color:#269575}.flow-condition .vue-flow__handle-right{right:58px;top:50%;background:#fff1ef;border-color:#bd6861}
</style>

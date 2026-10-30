import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import vm from 'node:vm'
import { parse as parseVue } from '@vue/compiler-sfc'
import { parse } from '@babel/parser'
import { parseCertificateTarget, formatCertificateTarget } from '../src/utils/certificateTarget.js'
import { probeTypes } from '../src/utils/probeTypes.js'

const source = parseVue(readFileSync(new URL('../src/views/inspection/InspectionTaskEditor.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const names = new Set(['defaultForm', 'selectedNode', 'copyTask', 'fillForm', 'configuredNodeValues', 'parseObject', 'objectToRows', 'objectToLabelRows', 'rowsToObject', 'selectedExecutionNodes', 'buildPayload', 'changeType'])
const code = parse(source, { sourceType: 'module' }).program.body.filter(node =>
  node.type === 'FunctionDeclaration' ? names.has(node.id.name) : node.type === 'VariableDeclaration' && node.declarations.some(d => names.has(d.id.name))
).map(node => source.slice(node.start, node.end)).join('\n')
function editor() {
  const context = vm.createContext({ parseCertificateTarget, formatCertificateTarget, taskTypes: probeTypes, computed: ({get,set}) => ({ get value() { return get() }, set value(value) { set(value) } }) })
  vm.runInContext('const legacyNodeValues = {value: []}; const automationTasks = {value: []}; const formRef = {value: null};', context)
  vm.runInContext(code + '\nconst form = defaultForm();', context)
  return expression => vm.runInContext(expression, context)
}
const original = {
  id: 8, group_id: 3, name: 'API', type: 'http', target: 'https://example.com',
  interval_seconds: 90, failure_threshold: 2, recovery_threshold: 4, severity: 'P1', fault_center_id: 5, status: 1,
  external_labels: '{"env":"production"}',
  config: JSON.stringify({ target: 'https://example.com', description: '生产环境接口', method: 'POST', headers: {Accept: 'application/json'}, params: {q: 'value'}, body: '{}', execution_nodes: [{node_type:'ssh', node_id:7}], timeout_ms: 12000 }),
}

test('single node selection replaces the previous selection in saved configuration', () => {
  const run = editor()
  run("selectedNode.value = 'ssh:7'; selectedNode.value = 'agent:9'")
  assert.deepEqual(JSON.parse(run('JSON.stringify(selectedExecutionNodes())')), [{node_type:'agent', node_id:9}])
  run("selectedNode.value = 'direct'")
  assert.deepEqual(JSON.parse(run('JSON.stringify(selectedExecutionNodes())')), [{node_type:'direct', node_id:0}])
})

test('copy creates a new draft in the original group with unique name and preserved configuration', () => {
  const run = editor()
  run(`copyTask(${JSON.stringify(original)}, [${JSON.stringify(original)}, {group_id:3,name:'API 副本'}])`)
  assert.equal(run('form.id'), null)
  assert.equal(run('form.name'), 'API 副本 2')
  const payload = JSON.parse(run('JSON.stringify(buildPayload())'))
  assert.equal(payload.group_id, 3)
  assert.equal(payload.fault_center_id, 5)
  assert.equal(payload.interval_seconds, 90)
  assert.equal(payload.severity, 'P1')
  assert.deepEqual(JSON.parse(payload.external_labels), {env:'production'})
  const config = JSON.parse(payload.config)
  assert.equal(config.description, '生产环境接口')
  assert.equal(config.method, 'POST')
  run("changeType('tcp')")
  assert.equal(run('form.description'), '生产环境接口')
  assert.deepEqual(config.headers, {Accept:'application/json'})
  assert.deepEqual(config.params, {q:'value'})
  assert.equal(config.body, '{}')
  assert.deepEqual(config.execution_nodes, [{node_type:'ssh',node_id:7}])
})

test('existing multi-node tasks are preserved, but their copies require a single node', () => {
  const run = editor()
  const row = {...original, config: JSON.stringify({target:original.target,execution_nodes:[{node_type:'direct',node_id:0},{node_type:'ssh',node_id:7}]})}
  run(`fillForm(${JSON.stringify(row)}); legacyNodeValues.value = [...form.node_values]`)
  assert.equal(run('selectedNode.value'), '__legacy__')
  assert.equal(run('selectedExecutionNodes().length'), 2)
  run("selectedNode.value = 'ssh:7'")
  assert.equal(run('selectedExecutionNodes().length'), 1)
  run("selectedNode.value = '__legacy__'")
  assert.equal(run('selectedExecutionNodes().length'), 2)
  run(`copyTask(${JSON.stringify(row)}, [])`)
  assert.equal(run('form.node_values.length'), 0)
  run("changeType('tcp')")
  assert.equal(run('form.node_values.length'), 0)
  run("selectedNode.value = 'direct'")
  assert.equal(run('selectedExecutionNodes().length'), 1)
})

 test('certificate target supports custom ports and preserves them when editing and copying', () => {
  const run = editor()
  const row = {...original, type:'certificate_expiry', target:'example.com', config:JSON.stringify({host:'example.com',port:8443,execution_nodes:[{node_type:'direct',node_id:0}]})}
  run(`fillForm(${JSON.stringify(row)})`)
  assert.equal(run('form.host'), 'example.com:8443')
  assert.equal(JSON.parse(run('buildPayload().config')).port, 8443)
  run(`copyTask(${JSON.stringify(row)}, [])`)
  assert.equal(JSON.parse(run('buildPayload().config')).port, 8443)
  run("form.host = 'example.com'")
  assert.equal(JSON.parse(run('buildPayload().config')).port, 443)
  assert.deepEqual(parseCertificateTarget('[::1]:8443'), {host:'::1',port:8443})
  for (const value of ['example.com:0','example.com:65536','example.com:abc','example.com/path','']) assert.throws(() => parseCertificateTarget(value))
})

test('script probes only serialize the automation task reference', () => {
  const run = editor()
  run("automationTasks.value = [{id: 12, name: '健康检查', status: 1}]; changeType('script'); Object.assign(form, {automation_task_id: 12, node_values: ['ssh:7'], timeout_ms: 100, script_id: 99})")
  const payload = JSON.parse(run('JSON.stringify(buildPayload())'))
  assert.equal(payload.target, '健康检查')
  const config = JSON.parse(payload.config)
  assert.equal(config.automation_task_id, 12)
  for (const field of ['script_id', 'execution_nodes', 'timeout_ms', 'node_type', 'node_id']) assert.equal(Object.hasOwn(config, field), false)
})

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import vm from 'node:vm'
import { parseCertificateTarget } from '../src/utils/certificateTarget.js'
import { probeTypes } from '../src/utils/probeTypes.js'
import { parse as parseVue } from '@vue/compiler-sfc'
import { parse } from '@babel/parser'

// Exercise the actual editor's defaults and request serialization without a DOM.
const source = parseVue(readFileSync(new URL('../src/views/inspection/InspectionTaskEditor.vue', import.meta.url), 'utf8')).descriptor.scriptSetup.content
const names = new Set(['taskTypes', 'defaultForm', 'changeType', 'selectedExecutionNodes', 'buildPayload', 'rowsToObject'])
const code = parse(source, { sourceType: 'module' }).program.body.filter(node =>
  node.type === 'FunctionDeclaration' ? names.has(node.id.name) : node.type === 'VariableDeclaration' && node.declarations.some(d => names.has(d.id.name))
).map(node => source.slice(node.start, node.end)).join('\n')
function editor() {
  const context = vm.createContext({ probeTypes, parseCertificateTarget })
  vm.runInContext(code + '\nconst form = defaultForm(); const formRef = { value: null };', context)
  return expression => vm.runInContext(expression, context)
}

test('certificate type defaults and custom policy survive saving', () => {
  const run = editor()
  run("changeType('certificate_expiry')")
  assert.equal(run('form.port'), 443)
  assert.equal(run('form.interval_seconds'), 21600)
  assert.equal(run('form.expiry_alert_days'), 30)
  assert.equal(run('form.failure_threshold'), 1)
  assert.equal(run('form.recovery_threshold'), 1)
  run("Object.assign(form, { host: 'example.com', name: '证书监控', expiry_alert_days: 14, interval_seconds: 43200, failure_threshold: 2, recovery_threshold: 2, server_name: 'example.com' })")
  const payload = JSON.parse(run('JSON.stringify(buildPayload())'))
  assert.equal(payload.type, 'certificate_expiry')
  assert.equal(payload.interval_seconds, 43200)
  assert.equal(payload.failure_threshold, 2)
  assert.equal(payload.recovery_threshold, 2)
  const config = JSON.parse(payload.config)
  assert.equal(config.expiry_alert_days, 14)
  assert.equal(config.server_name, 'example.com')
  assert.deepEqual(config.execution_nodes, [{ node_type: 'direct', node_id: 0 }])
})

test('TCP remains a connectivity task without an expiry policy', () => {
  const run = editor()
  run("changeType('tcp'); form.host = 'example.com'")
  assert.equal(run('form.interval_seconds'), 300)
  assert.equal(run("Object.hasOwn(JSON.parse(buildPayload().config), 'expiry_alert_days')"), false)
})

 test('retired probe types are not offered in the task editor', () => {
 const run = editor()
 assert.deepEqual(JSON.parse(run('JSON.stringify(taskTypes.map(item => item.value))')), ['http', 'icmp', 'tcp', 'certificate_expiry', 'script'])
 })

test('certificate probe serialization keeps its selected execution node', () => {
  const run = editor()
  run("changeType('certificate_expiry'); form.host = 'example.com'; form.node_values = ['agent:7']; form.expiry_alert_days = 14")
  const payload = JSON.parse(run('JSON.stringify(buildPayload())'))
  const config = JSON.parse(payload.config)
  assert.equal(payload.type, 'certificate_expiry')
  assert.equal(config.expiry_alert_days, 14)
  assert.deepEqual(config.execution_nodes, [{node_type: 'agent', node_id: 7}])
  run("changeType('tcp')")
  assert.equal(run("Object.hasOwn(JSON.parse(buildPayload().config), 'expiry_alert_days')"), false)
})

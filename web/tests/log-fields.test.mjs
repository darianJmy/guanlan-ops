import test from 'node:test'
import assert from 'node:assert/strict'
import { flattenLogFields, formatLogField } from '../src/components/explore/log-fields.mjs'
test('ES fields expose nested paths without losing scalar types or arrays', () => {
 const source = { service: { name: 'api' }, status: 0, error: false, nullable: null, tags: ['a','b'], empty: {}, 'service.name': 'literal' }
 const fields = flattenLogFields(source)
 assert.equal(fields['service.name'], 'literal')
 assert.equal(formatLogField(fields.status), '0')
 assert.equal(formatLogField(fields.error), 'false')
 assert.equal(formatLogField(fields.nullable), 'null')
 assert.equal(formatLogField(fields.tags), '["a","b"]')
 assert.equal(formatLogField(fields.empty), '{}')
 assert.equal(formatLogField(fields.missing), '—')
 assert.equal(source.service.name, 'api')
})
test('untrusted field names and HTML remain data', () => {
 const fields=flattenLogFields(JSON.parse('{"__proto__":"safe","message":"<script>alert(1)</script>"}'))
 assert.equal(Object.getPrototypeOf(fields), null)
 assert.equal(fields.__proto__, 'safe')
 assert.equal(formatLogField(fields.message), '<script>alert(1)</script>')
})

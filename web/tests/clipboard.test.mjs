import test from 'node:test'
import assert from 'node:assert/strict'
import { copyText } from '../src/utils/clipboard.js'

test('copy supports clipboard API, HTTP fallback, and denied permission inside drawers', async t => {
  for (const mode of ['api', 'unavailable', 'denied', 'failed']) {
    await t.test(mode, async t => {
      let copied, parent, removed = false, restored = false
      const input = { style: {}, focus() {}, select() {}, setSelectionRange() {}, remove() { removed = true } }
      const drawer = { appendChild(element) { parent = drawer; assert.equal(element, input) } }
      const originalNavigator = Object.getOwnPropertyDescriptor(globalThis, 'navigator')
      Object.defineProperty(globalThis, 'navigator', { configurable: true, value: { clipboard: mode === 'unavailable' || mode === 'failed' ? undefined : { async writeText(text) { if (mode === 'denied') throw new Error('denied'); copied = text } } } })
      const originalDocument = Object.getOwnPropertyDescriptor(globalThis, 'document')
      Object.defineProperty(globalThis, 'document', { configurable: true, value: {
        body: { appendChild() { assert.fail('must use drawer container') } },
        activeElement: { focus() { restored = true } },
        createElement() { return input },
        execCommand(command) { assert.equal(command, 'copy'); copied = input.value; return mode !== 'failed' },
      } })
      t.after(() => { if (originalNavigator) Object.defineProperty(globalThis, 'navigator', originalNavigator); else delete globalThis.navigator; if (originalDocument) Object.defineProperty(globalThis, 'document', originalDocument); else delete globalThis.document })
      const text = '{"message":"中文复制内容"}'
      if (mode === 'failed') await assert.rejects(copyText(text, drawer), /Copy command failed/)
      else await copyText(text, drawer)
      assert.equal(copied, text)
      if (mode !== 'api') { assert.equal(parent, drawer); assert.ok(removed); assert.ok(restored) }
    })
  }
})

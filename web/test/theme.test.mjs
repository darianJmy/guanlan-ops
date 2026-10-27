import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { themes, isTheme, resolveTheme, applyTheme } from '../src/theme/index.js'

test('day and night palettes resolve and apply with the right color scheme', () => {
  const classes = new Set()
  globalThis.window = { matchMedia: () => ({ matches: false }) }
  globalThis.document = { documentElement: { dataset: {}, style: {}, classList: { toggle(name, enabled) { enabled ? classes.add(name) : classes.delete(name) } } } }
  assert.deepEqual(themes.map(theme => theme.id), ['light', 'dark'])
  for (const theme of [...themes, ...themes.toReversed()]) {
    assert.ok(isTheme(theme.id))
    applyTheme(theme.id)
    assert.equal(document.documentElement.dataset.theme, theme.id)
    assert.equal(classes.has('dark'), theme.dark)
    assert.equal(document.documentElement.style.colorScheme, theme.dark ? 'dark' : 'light')
  }
  assert.equal(resolveTheme('system', true), 'dark')
  assert.equal(resolveTheme('system', false), 'light')
  assert.equal(resolveTheme('unknown', true), 'light')
  assert.equal(isTheme('unknown'), false)
  assert.equal(isTheme('system'), false)
})

const css = readFileSync(new URL('../src/styles/console.css', import.meta.url), 'utf8')
function luminance(hex) {
  const rgb = hex.match(/[a-f\d]{2}/gi).map(value => parseInt(value, 16) / 255).map(value => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4)
  return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722
}
function contrast(a, b) {
  const values = [luminance(a), luminance(b)].sort((a, b) => b - a)
  return (values[0] + .05) / (values[1] + .05)
}
test('body, secondary and button text have readable contrast in every palette', () => {
  for (const theme of themes) {
    const selector = theme.id === 'dark' ? 'html.dark' : `html[data-theme="${theme.id}"]`
    const block = css.slice(css.indexOf(selector)).split('}')[0]
    const tokens = Object.fromEntries([...block.matchAll(/--ops-([\w-]+): (#[\da-f]{6});/g)].map(match => [match[1], match[2]]))
    for (const background of ['bg', 'surface', 'surface-soft']) {
      for (const foreground of ['text', 'muted']) assert.ok(contrast(tokens[foreground], tokens[background]) >= 4.5, `${theme.id} ${foreground}/${background}`)
    }
    assert.ok(contrast(tokens['on-primary'], tokens.primary) >= 4.5, `${theme.id} button`)
  }
})

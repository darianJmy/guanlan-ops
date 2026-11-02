export async function copyText(text, container = document.body) {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return
    } catch { /* Fall back when clipboard permissions are unavailable. */ }
  }

  const previousFocus = document.activeElement
  const input = document.createElement('textarea')
  input.value = text
  input.readOnly = true
  input.style.cssText = 'position:fixed;left:0;top:0;width:1px;height:1px;opacity:0;pointer-events:none;font-size:16px;'
  // Keep the temporary input inside the drawer's focus trap.
  container.appendChild(input)
  try {
    input.focus({ preventScroll: true })
    input.select()
    input.setSelectionRange(0, text.length)
    if (!document.execCommand('copy')) throw new Error('Copy command failed')
  } finally {
    input.remove()
    previousFocus?.focus({ preventScroll: true })
  }
}

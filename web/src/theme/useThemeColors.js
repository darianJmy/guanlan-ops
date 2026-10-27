import { onBeforeUnmount, onMounted, shallowRef } from 'vue'

// Canvas charts and editors need resolved colors instead of CSS variable references.
export function useThemeColors() {
  const read = () => {
    const css = getComputedStyle(document.documentElement)
    return Object.fromEntries(['text', 'muted', 'border', 'border-soft', 'surface', 'primary', 'success', 'danger'].map(key => [key, css.getPropertyValue(`--ops-${key}`).trim()]))
  }
  const colors = shallowRef(read())
  const observer = new MutationObserver(() => { colors.value = read() })
  onMounted(() => {
    colors.value = read()
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'class'] })
  })
  onBeforeUnmount(() => observer.disconnect())
  return colors
}

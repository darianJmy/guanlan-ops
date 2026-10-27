export const themes = [
  { id: 'light', label: '浅色模式', dark: false },
  { id: 'dark', label: '深色模式', dark: true },
]
export const isTheme = value => themes.some(theme => theme.id === value)

export function resolveTheme(value, systemDark = window.matchMedia('(prefers-color-scheme: dark)').matches) {
  if (isTheme(value)) return value
  if (value === 'system') return systemDark ? 'dark' : 'light'
  return 'light'
}
export function applyTheme(value) {
  const resolved = resolveTheme(value)
  const dark = themes.find(theme => theme.id === resolved).dark
  document.documentElement.dataset.theme = resolved
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
}

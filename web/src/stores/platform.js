import { defineStore } from 'pinia'
import { getPublicSettings } from '../api/systemSetting'

import { isTheme, resolveTheme, applyTheme } from '../theme'

const themeStorageKey = 'aiops_theme_preference'

function savedThemePreference() {
  try {
    const value = localStorage.getItem(themeStorageKey)
    return isTheme(value) ? value : ''
  } catch { return '' }
}

export const usePlatformStore = defineStore('platform', {
  state: () => ({ systemName:'观澜运维平台', systemSubtitle:'智能运维控制台', logoData:'', defaultTheme:resolveTheme('system'), themePreference:savedThemePreference(), localEnabled:true, ldapEnabled:false, oidcEnabled:false, loaded:false }),
  actions: {
    async load() {
      try {
        const data=(await getPublicSettings()).data||{}
        this.systemName=data.system_name||'观澜运维平台'; this.systemSubtitle=data.system_subtitle||''; this.logoData=data.logo_data||''; this.defaultTheme=resolveTheme(data.default_theme||'system'); document.title=this.systemName; this.applyFavicon()
        this.localEnabled=data.local_enabled!==false; this.ldapEnabled=Boolean(data.ldap_enabled); this.oidcEnabled=Boolean(data.oidc_enabled)
      } finally { this.applyTheme(); this.loaded=true }
    },
    applyFavicon() {
      const icon = document.querySelector('link[rel="icon"]')
      if (!icon) return
      icon.href = this.logoData || '/favicon.svg'
      if (this.logoData) icon.removeAttribute('type')
      else icon.type = 'image/svg+xml'
    },
    applyTheme() {
      applyTheme(this.themePreference || this.defaultTheme)
    },
    setThemePreference(theme) {
      if (!isTheme(theme)) return
      this.themePreference=theme
      try { localStorage.setItem(themeStorageKey,theme) } catch {}
      this.applyTheme()
    },
    setSettings(data) { this.systemName=data.system_name; this.systemSubtitle=data.system_subtitle; this.logoData=data.logo_data||''; this.defaultTheme=resolveTheme(data.default_theme); document.title=this.systemName; this.applyFavicon(); this.applyTheme() },
  },
})

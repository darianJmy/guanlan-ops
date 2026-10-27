import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './styles/console.css'
import { applyTheme } from './theme'

try { applyTheme(localStorage.getItem('aiops_theme_preference') || 'system') } catch { applyTheme('system') }
import zhCn from 'element-plus/dist/locale/zh-cn.mjs'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import TableEmpty from './components/TableEmpty.vue'
import router from './router'
import permission from './directives/permission'

const app = createApp(App)

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus, { locale: zhCn })
app.component('TableEmpty', TableEmpty)
app.use(createPinia())
app.use(router)
app.directive('permission', permission)

app.mount('#app')

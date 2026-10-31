<template>
  <section class="settings-workspace">
    <aside class="settings-sidebar">
      <nav class="settings-navigation" aria-label="系统设置分类">
        <router-link v-for="item in items" :key="item.path" :to="item.path"
          class="settings-navigation-item" :class="{ active: route.path === item.path }"
          :aria-current="route.path === item.path ? 'page' : undefined">
          <span>{{ item.name }}</span>
        </router-link>
      </nav>
    </aside>
    <div class="settings-workspace-content">
      <div class="settings-content-body"><slot /></div>
    </div>
  </section>
</template>

<script setup>
import { useRoute } from 'vue-router'
defineProps({ items: { type: Array, default: () => [] } })
const route = useRoute()
</script>

<style scoped>
.settings-workspace {
  display: grid;
  grid-template-columns: 164px minmax(0, 1fr);
  min-width: 0;
  min-height: calc(100vh - 112px);
}
.settings-sidebar {
  padding: 30px 8px 24px 0;
}
.settings-navigation {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.settings-navigation-item {
  display: flex;
  align-items: center;
  min-height: 42px;
  box-sizing: border-box;
  padding: 10px 18px;
  color: var(--ops-muted);
  font-family: var(--ops-font-family);
  font-size: 14px;
  line-height: 22px;
  text-decoration: none;
  border-radius: 7px;
  transition: color .15s, background-color .15s;
}
.settings-navigation-item:hover { color: var(--ops-text); background: var(--ops-surface); }
.settings-navigation-item.active {
  color: var(--ops-primary);
  background: var(--ops-surface);
  font-weight: 600;
}
.settings-navigation-item:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 3px; }
.settings-workspace-content { min-width: 0; padding: 30px 36px 40px; }
.settings-content-body { max-width: 980px; min-width: 0; margin: 0 auto; }
@media (max-width: 1000px) {
  .settings-workspace { grid-template-columns: 132px minmax(0, 1fr); }
  .settings-sidebar { padding: 26px 6px 22px 0; }
  .settings-workspace-content { padding: 26px 22px; }
}
@media (max-width: 640px) {
  .settings-workspace { grid-template-columns: 92px minmax(0, 1fr); }
  .settings-sidebar { padding: 20px 4px 20px 0; }
  .settings-navigation-item { padding: 11px 10px; font-size: 13px; }
  .settings-workspace-content { padding: 20px 14px; }
}
@media (prefers-reduced-motion: reduce) { .settings-navigation-item { transition: none; } }
</style>

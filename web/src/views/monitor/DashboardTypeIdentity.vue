<template>
  <div class="dashboard-type-identity">
    <span class="identity-logo-slot" :class="{ 'is-wide': wide }"><img :src="icon" :class="{ 'is-wide': wide }" alt="" /></span>
    <h2>{{ label }}</h2>
  </div>
</template>

<script setup>
defineProps({
  icon: { type: String, required: true },
  label: { type: String, required: true },
  wide: Boolean,
})
</script>

<style scoped>
.dashboard-type-identity { display: flex; align-items: center; gap: 16px; min-height: 52px; margin-bottom: 24px; padding: 0 0 18px; border-bottom: 1px solid var(--ops-border-soft); }
.identity-logo-slot { display: flex; align-items: center; justify-content: center; width: 36px; height: 36px; flex: none; }
.identity-logo-slot.is-wide { width: 88px; }
.dashboard-type-identity img { display: block; width: 36px; height: 36px; flex: none; object-fit: contain; }
.dashboard-type-identity img.is-wide { width: 88px; height: 32px; }
.dashboard-type-identity h2 { margin: 0; color: var(--ops-text); font-size: 16px; font-weight: 600; line-height: 1.5; }
</style>

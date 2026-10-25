<template>
  <LogExplorer ref="view" :source="source" />
</template>
<script setup>
import { ref } from 'vue'
import LogExplorer from '../../components/explore/LogExplorer.vue'
defineProps({ source: { type: Object, required: true } })
const view = ref(null)
defineExpose({ openQueryHistory: () => view.value?.openQueryHistory() })
</script>

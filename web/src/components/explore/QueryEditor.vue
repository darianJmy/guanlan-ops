<template><div class="editor"><CodeEditor v-model="value" :language="language" height="126px" aria-label="查询表达式" @execute="$emit('execute')" /></div></template>
<script setup>
import { computed, defineAsyncComponent } from 'vue'
const CodeEditor = defineAsyncComponent(() => import('../CodeEditor.vue'))
const props = defineProps({ modelValue: { type:String,default:'' }, language: {type:String,default:'promql'} })
const emit = defineEmits(['update:modelValue','execute'])
const value = computed({ get:()=>props.modelValue,set:value=>emit('update:modelValue',value) })
</script>
<style scoped>.editor :deep(.code-editor){border:0;border-radius:0}.editor{min-height:126px}</style>

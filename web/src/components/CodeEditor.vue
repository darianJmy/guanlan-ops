<template>
  <div ref="container" class="code-editor" :style="{ height }" />
</template>

<script setup>
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useThemeColors } from '../theme/useThemeColors'
import * as monaco from 'monaco-editor/editor/editor.api'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'
import 'monaco-editor/languages/definitions/python/register'
import 'monaco-editor/languages/definitions/shell/register'
import 'monaco-editor/languages/definitions/sql/register'

const props = defineProps({
  modelValue: { type: String, default: '' },
  language: { type: String, default: 'shell' },
  height: { type: String, default: '420px' },
  ariaLabel: { type: String, default: '代码编辑器' },
})
const emit = defineEmits(['update:modelValue', 'execute'])
const container = ref()
const themeColors = useThemeColors()
function updateEditorTheme() {
  monaco.editor.defineTheme('guanlan', {
    base: document.documentElement.classList.contains('dark') ? 'vs-dark' : 'vs',
    inherit: true,
    rules: [],
    colors: {
      'editor.background': themeColors.value.surface,
      'editor.foreground': themeColors.value.text,
      'editorLineNumber.foreground': themeColors.value.muted,
      'editorCursor.foreground': themeColors.value.primary,
    },
  })
  monaco.editor.setTheme('guanlan')
}
watch(themeColors, updateEditorTheme)
let editor
let contentSubscription

self.MonacoEnvironment = { getWorker: () => new EditorWorker() }

if (!monaco.languages.getLanguages().some(item => item.id === 'yaml')) {
  monaco.languages.register({ id: 'yaml', extensions: ['.yaml', '.yml'], aliases: ['YAML', 'yaml'] })
  monaco.languages.setMonarchTokensProvider('yaml', {
    tokenizer: {
      root: [
        [/^\s*#.*/, 'comment'],
        [/^([\w.-]+)(\s*:)/, ['key', 'delimiter']],
        [/\b(true|false|null)\b/, 'keyword'],
        [/-?\d+(\.\d+)?/, 'number'],
        [/"([^"\\]|\\.)*"/, 'string'],
        [/'[^']*'/, 'string'],
      ],
    },
  })
}

for (const language of ['promql', 'logql']) {
  if (!monaco.languages.getLanguages().some(item => item.id === language)) {
    monaco.languages.register({ id: language })
    monaco.languages.setMonarchTokensProvider(language, { tokenizer: { root: [
      [/#.*$/, 'comment'], [/"([^"\\]|\\.)*"/, 'string'],
      [/\b(sum|avg|min|max|count|rate|irate|increase|by|without|on|ignoring|group_left|group_right|bool|and|or|unless|json|logfmt|line_format|unwrap|count_over_time|sum_over_time)\b/, 'keyword'],
      [/\d+(?:\.\d+)?(?:ms|s|m|h|d|w|y)?/, 'number'], [/[{}()[\]]/, '@brackets'], [/[=!~|+*/<>-]+/, 'operator'], [/[a-zA-Z_:][\w:]*/, 'identifier'],
    ] } })
    monaco.languages.registerCompletionItemProvider(language, { provideCompletionItems(model, position) {
      const word = model.getWordUntilPosition(position), range = { startLineNumber: position.lineNumber, endLineNumber: position.lineNumber, startColumn: word.startColumn, endColumn: word.endColumn }
      const words = language === 'promql' ? ['sum','avg','max','min','count','rate','irate','increase','histogram_quantile','by','without'] : ['json','logfmt','line_format','unwrap','count_over_time','rate']
      return { suggestions: words.map(word => ({ label:word, kind:monaco.languages.CompletionItemKind.Function, insertText:word, range })) }
    } })
  }
}

onMounted(() => {
  updateEditorTheme()
  editor = monaco.editor.create(container.value, {
    value: props.modelValue,
    language: props.language,
    theme: 'guanlan',
    automaticLayout: true,
    ariaLabel: props.ariaLabel,
    fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
    fontSize: 13,
    lineHeight: 21,
    minimap: { enabled: false },
    padding: { top: 12, bottom: 12 },
    scrollBeyondLastLine: false,
    tabSize: 2,
    insertSpaces: true,
    wordWrap: 'on',
  })
  editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => emit('execute'))
  contentSubscription = editor.onDidChangeModelContent(() => emit('update:modelValue', editor.getValue()))
})

watch(() => props.modelValue, value => {
  if (editor && value !== editor.getValue()) editor.setValue(value || '')
})

watch(() => props.language, language => {
  const model = editor?.getModel()
  if (model) monaco.editor.setModelLanguage(model, language || 'plaintext')
})

onBeforeUnmount(() => {
  contentSubscription?.dispose()
  const model = editor?.getModel()
  editor?.dispose()
  model?.dispose()
})
</script>

<style scoped>
.code-editor { width: 100%; overflow: hidden; border: 1px solid var(--ops-border); border-radius: 7px; }
.code-editor:focus-within { border-color: var(--ops-primary); box-shadow: 0 0 0 1px var(--ops-primary) inset; }
</style>

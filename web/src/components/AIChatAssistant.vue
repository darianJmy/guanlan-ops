<template>
  <section class="ai-chat-panel" aria-label="观澜运维平台 AI 助手">
    <header class="chat-header">
      <div class="chat-header-info">
        <button class="chat-header-button" type="button" aria-label="会话历史" title="会话历史" :aria-expanded="historyVisible" @click="historyVisible = !historyVisible">
          <svg xmlns="http://www.w3.org/2000/svg" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" aria-hidden="true">
            <rect width="18" height="18" x="3" y="3" rx="2" />
            <path d="M9 3v18" />
          </svg>
        </button>
        <div class="chat-title">
          <strong title="AI 助手">AI 助手</strong>
          <small v-if="sending">正在分析…</small>
        </div>
      </div>
      <div class="chat-header-actions">
        <el-select v-model="selectedModelID" class="chat-model-select" placeholder="模型" aria-label="模型" :disabled="sending || loadingMessages || loadingModels || !availableModels.length" :loading="loadingModels">
          <el-option v-for="model in availableModels" :key="model.id" :label="model.name" :value="model.id"><span>{{ model.name }}</span><small style="float:right;margin-left:12px;color:var(--ops-muted)">{{ model.provider }}</small></el-option>
        </el-select>
        <button class="chat-header-button" type="button" aria-label="新会话" title="新会话" :disabled="sending || loadingMessages" @click="newConversation">
          <svg xmlns="http://www.w3.org/2000/svg" width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" aria-hidden="true">
            <path d="M12 5v14M5 12h14" />
          </svg>
        </button>
      </div>
    </header>
    <div v-if="!loadingModels && availableModels.length && !selectedModelID" class="chat-model-notice" role="status">
      <small>原模型已停用或删除，请重新选择。</small>
    </div>
    <section v-if="historyVisible" class="chat-history" aria-label="会话历史列表">
      <div class="history-search">
        <el-input v-model="historySearch" placeholder="搜索会话" aria-label="搜索会话" clearable :prefix-icon="Search" />
      </div>
      <div v-loading="loadingConversations" class="conversation-list">
        <div v-for="conversation in filteredConversations" :key="conversation.id" class="conversation-item" :class="{ active: conversation.id === activeConversationID }">
          <button type="button" class="conversation-copy" :disabled="sending || loadingMessages" :aria-current="conversation.id === activeConversationID ? 'true' : undefined" @click="selectConversation(conversation)">
            <strong>{{ conversation.title || '新对话' }}</strong>
            <small>{{ formatConversationTime(conversation.updated_at) }}</small>
          </button>
          <el-button link type="danger" :icon="Delete" aria-label="删除对话" :disabled="sending || loadingMessages" @click="removeConversation(conversation)" />
        </div>
        <el-empty v-if="!loadingConversations && !filteredConversations.length" :image-size="56" :description="historySearch ? '没有匹配的会话' : '暂无历史对话'" />
      </div>
    </section>
    <div v-show="!historyVisible" class="chat-main">
        <section v-if="!activeConversationID && !messages.length" class="chat-welcome">
          <span class="welcome-icon" aria-hidden="true">
            <svg xmlns="http://www.w3.org/2000/svg" width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
              <path d="M12 8V4H8" />
              <rect width="16" height="12" x="4" y="8" rx="2" />
              <path d="M2 14h2m16 0h2m-7-1v2m-6-2v2" />
            </svg>
          </span>
          <h2>有什么可以帮你？</h2>
        </section>

        <section v-else ref="messageContainer" v-loading="loadingMessages" class="messages-container" aria-live="polite">
          <article v-for="message in messages" :key="message.id" class="message" :class="message.role">
            <span class="message-avatar" aria-hidden="true">
              <User v-if="message.role === 'user'" />
              <svg v-else xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                <path d="M12 8V4H8" />
                <rect width="16" height="12" x="4" y="8" rx="2" />
                <path d="M2 14h2m16 0h2m-7-1v2m-6-2v2" />
              </svg>
            </span>
            <div class="message-body">
              <div class="message-meta">
                <strong>{{ message.role === 'user' ? '您' : 'AI 助手' }}</strong>
                <time>{{ formatMessageTime(message.created_at) }}</time>
              </div>
              <div v-if="message.role === 'assistant'" class="message-content markdown-body" v-html="renderMarkdown(message.content)" />
              <div v-else class="message-content user-content">{{ message.content }}</div>
              <AIExecutionTrace v-if="message.role === 'assistant'" :metadata="message.metadata" />
            </div>
          </article>
          <article v-if="sending" class="message assistant is-thinking">
            <span class="message-avatar" aria-hidden="true"><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-linejoin="round" stroke-width="2">
                <path d="M12 8V4H8" />
                <rect width="16" height="12" x="4" y="8" rx="2" />
                <path d="M2 14h2m16 0h2m-7-1v2m-6-2v2" />
              </svg></span>
            <div class="message-body">
              <div class="message-meta"><strong>AI 助手</strong><span>正在处理</span></div>
              <div class="thinking-dots" aria-label="AI 正在思考"><i /><i /><i /></div>
            </div>
          </article>
        </section>

        <footer class="input-area">
          <div class="input-shell">
            <el-input
              ref="inputRef"
              v-model="input"
              type="textarea"
              :rows="3"
              resize="none"
              maxlength="4000"
              show-word-limit
              placeholder="询问任何关于平台和可观测性的问题..."
              :disabled="sending || loadingMessages"
              @keydown="handleKeydown"
            />
            <el-button
              class="send-button"
              type="primary"
              circle
              :icon="Promotion"
              :loading="sending"
              :disabled="!input.trim() || loadingMessages || !selectedModelID"
              aria-label="发送消息"
              @click="send"
            />
          </div>
          <small>按 Enter 发送，Shift + Enter 换行 · 对话仅当前用户可见</small>
        </footer>
    </div>
  </section>
</template>

<script setup>
import AIExecutionTrace from './AIExecutionTrace.vue'
import { getChatModels } from '../api/aiModel'
import { computed, nextTick, ref, watch } from 'vue'
import { Delete, Search, Promotion, User } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { deleteConversation, getConversations, getMessages, sendMessage } from '../api/chat'

const props = defineProps({ visible: Boolean })
defineEmits(['close'])
const availableModels = ref([])
const selectedModelID = ref(null)
const loadingModels = ref(false)
const historyVisible = ref(false)
const historySearch = ref('')
const conversations = ref([])
const activeConversationID = ref(0)
const messages = ref([])
const input = ref('')
const sending = ref(false)
const loadingConversations = ref(false)
const loadingMessages = ref(false)
const messageContainer = ref()
const inputRef = ref()

const filteredConversations = computed(() => conversations.value.filter(item => (item.title || '新对话').toLowerCase().includes(historySearch.value.trim().toLowerCase())))

let initialized = false

watch(() => props.visible, (visible) => { if (visible) openChat() }, { immediate: true })

async function openChat() {
  if (loadingConversations.value) return
  const firstOpen = !initialized
  initialized = true
  await Promise.all([loadConversations(), loadModels()])
  if (firstOpen && !sending.value && !input.value.trim() && !activeConversationID.value && !messages.value.length && conversations.value.length) {
    await selectConversation(conversations.value[0])
  }
  nextTick(() => { if (props.visible && !historyVisible.value) inputRef.value?.focus() })
}

async function loadModels() {
  loadingModels.value = true
  try {
    availableModels.value = (await getChatModels()).data || []
    if (!availableModels.value.some(item => item.id === selectedModelID.value)) selectedModelID.value = activeConversationID.value ? null : availableModels.value[0]?.id || null
  } catch { availableModels.value = []; selectedModelID.value = null }
  finally { loadingModels.value = false }
}

async function loadConversations() {
  loadingConversations.value = true
  try {
    conversations.value = (await getConversations()).data || []
  } catch {
    ElMessage.error('读取对话列表失败')
  } finally {
    loadingConversations.value = false
  }
}

function newConversation() {
  if (sending.value || loadingMessages.value) return
  historyVisible.value = false
  activeConversationID.value = 0
  if (!selectedModelID.value) selectedModelID.value = availableModels.value[0]?.id || null
  messages.value = []
  input.value = ''
  nextTick(() => { if (props.visible && !historyVisible.value) inputRef.value?.focus() })
}

async function selectConversation(conversation) {
  if (sending.value || loadingMessages.value) return
  if (conversation.id === activeConversationID.value) { historyVisible.value = false; return }
  loadingMessages.value = true
  try {
    const result = await getMessages(conversation.id)
    activeConversationID.value = conversation.id
    selectedModelID.value = availableModels.value.some(item => item.id === conversation.model_id) ? conversation.model_id : (conversation.model_id ? null : availableModels.value[0]?.id || null)
    messages.value = result.data || []
    historyVisible.value = false
    await scrollToBottom()
  } catch {
    ElMessage.error('读取对话消息失败')
  } finally {
    loadingMessages.value = false
  }
}

async function removeConversation(conversation) {
  if (sending.value || loadingMessages.value) return
  try {
    await ElMessageBox.confirm(`确定删除“${conversation.title || '新对话'}”吗？`, '删除对话', { type: 'warning' })
    await deleteConversation(conversation.id)
    if (activeConversationID.value === conversation.id) newConversation()
    await loadConversations()
    ElMessage.success('对话已删除')
  } catch (error) {
    if (error !== 'cancel' && error !== 'close') ElMessage.error('删除对话失败')
  }
}

function handleKeydown(event) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return
  event.preventDefault()
  send()
}

async function send() {
  const content = input.value.trim()
  if (!content || sending.value || loadingMessages.value) return
  if (!selectedModelID.value) { ElMessage.warning("请先选择可用的 AI 模型"); return }

  input.value = ''
  sending.value = true
  const optimisticID = `local-${Date.now()}`
  messages.value.push({ id: optimisticID, role: 'user', content, created_at: new Date().toISOString() })
  await scrollToBottom()

  try {
    const response = await sendMessage({ conversation_id: activeConversationID.value || 0, message: content, model_id: selectedModelID.value })
    activeConversationID.value = response.data.conversation_id
    const [messageResult, conversationResult] = await Promise.all([
      getMessages(activeConversationID.value),
      getConversations(),
    ])
    messages.value = messageResult.data || []
    conversations.value = conversationResult.data || []
  } catch {
    messages.value = messages.value.filter(item => item.id !== optimisticID)
    input.value = content
    await loadModels()
    ElMessage.error('发送失败，请检查所选模型后重试')
  } finally {
    sending.value = false
    await scrollToBottom()
    nextTick(() => { if (props.visible && !historyVisible.value) inputRef.value?.focus() })
  }
}

async function scrollToBottom() {
  await nextTick()
  if (messageContainer.value) messageContainer.value.scrollTop = messageContainer.value.scrollHeight
}

function formatConversationTime(value) {
  if (!value) return ''
  const date = new Date(value)
  const now = new Date()
  if (date.toDateString() === now.toDateString()) return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  return date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
}

function formatMessageTime(value) {
  if (!value) return ''
  return new Date(value).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[char]))
}

function renderInline(value) {
  let html = escapeHTML(value)
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
  html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>')
  return html
}

function isTableSeparator(line) {
  return /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?\s*$/.test(line)
}

function tableCells(line) {
  return line.trim().replace(/^\||\|$/g, '').split('|').map(cell => cell.trim())
}

function renderMarkdown(value) {
  const lines = String(value ?? '').replace(/\r\n/g, '\n').split('\n')
  const output = []
  let index = 0
  while (index < lines.length) {
    const line = lines[index]
    if (line.trim().startsWith('```')) {
      const code = []
      index += 1
      while (index < lines.length && !lines[index].trim().startsWith('```')) code.push(lines[index++])
      output.push(`<pre><code>${escapeHTML(code.join('\n'))}</code></pre>`)
      index += 1
      continue
    }
    if (index + 1 < lines.length && line.includes('|') && isTableSeparator(lines[index + 1])) {
      const headers = tableCells(line)
      index += 2
      const rows = []
      while (index < lines.length && lines[index].includes('|') && lines[index].trim()) rows.push(tableCells(lines[index++]))
      output.push(`<div class="markdown-table-wrap"><table><thead><tr>${headers.map(cell => `<th>${renderInline(cell)}</th>`).join('')}</tr></thead><tbody>${rows.map(row => `<tr>${row.map(cell => `<td>${renderInline(cell)}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`)
      continue
    }
    if (/^\s*[-*]\s+/.test(line)) {
      const items = []
      while (index < lines.length && /^\s*[-*]\s+/.test(lines[index])) items.push(lines[index++].replace(/^\s*[-*]\s+/, ''))
      output.push(`<ul>${items.map(item => `<li>${renderInline(item)}</li>`).join('')}</ul>`)
      continue
    }
    if (/^\s*\d+\.\s+/.test(line)) {
      const items = []
      while (index < lines.length && /^\s*\d+\.\s+/.test(lines[index])) items.push(lines[index++].replace(/^\s*\d+\.\s+/, ''))
      output.push(`<ol>${items.map(item => `<li>${renderInline(item)}</li>`).join('')}</ol>`)
      continue
    }
    const heading = line.match(/^(#{1,4})\s+(.+)$/)
    if (heading) {
      const level = Math.min(heading[1].length + 2, 6)
      output.push(`<h${level}>${renderInline(heading[2])}</h${level}>`)
    } else if (/^\s*---+\s*$/.test(line)) {
      output.push('<hr>')
    } else if (/^\s*>\s?/.test(line)) {
      output.push(`<blockquote>${renderInline(line.replace(/^\s*>\s?/, ''))}</blockquote>`)
    } else if (line.trim()) {
      output.push(`<p>${renderInline(line)}</p>`)
    }
    index += 1
  }
  return output.join('')
}
</script>

<style scoped>
.ai-chat-panel{height:100%;min-height:0;display:flex;flex-direction:column;overflow:hidden;background:var(--ops-surface);color:var(--ops-text)}
.chat-header{display:flex;flex:none;min-height:64px;box-sizing:border-box;align-items:center;justify-content:space-between;gap:12px;padding:12px 16px;border-bottom:1px solid var(--ops-border-soft)}
.chat-header-info{display:flex;flex:1 0 auto;min-width:max-content;align-items:center;gap:8px}
.chat-header-actions{display:flex;flex:0 1 136px;min-width:112px;align-items:center;gap:4px}
.chat-header-button{display:grid;place-items:center;flex:0 0 32px;width:32px;height:32px;padding:0;border:0;border-radius:6px;background:transparent;color:var(--ops-text);cursor:pointer}
.chat-header-button:hover:not(:disabled){background:var(--ops-surface-soft);color:var(--ops-primary)}
.chat-header-button:focus-visible{outline:2px solid var(--ops-primary);outline-offset:2px}
.chat-header-button:disabled{opacity:.45;cursor:not-allowed}
.chat-model-select{width:100px;min-width:0;flex:1}
.chat-model-select :deep(.el-select__wrapper){min-height:32px}
.chat-title{display:flex;min-width:0;flex:1;flex-direction:column;gap:4px}
.chat-title strong{overflow:hidden;font-size:14px;font-weight:600;text-overflow:ellipsis;white-space:nowrap}
.chat-title small{overflow:hidden;font-size:12px;color:var(--ops-muted);text-overflow:ellipsis;white-space:nowrap}
.chat-main{display:flex;flex:1;min-height:0;min-width:0;flex-direction:column;background:var(--ops-bg)}
.chat-history{display:flex;flex:1;min-height:0;flex-direction:column}
.history-search{padding:12px 14px;border-bottom:1px solid var(--ops-border-soft)}
.conversation-list{flex:1;min-height:0;overflow:auto;padding:8px}
.conversation-item{display:flex;align-items:center;gap:10px;padding:6px 10px;margin-bottom:4px;border:1px solid transparent;border-radius:7px}
.conversation-item:hover,.conversation-item.active{background:var(--ops-primary-soft);border-color:var(--ops-border-soft)}
.conversation-copy{display:flex;flex:1;min-width:0;flex-direction:column;gap:6px;padding:8px 0;color:inherit;text-align:left;background:transparent;border:0;cursor:pointer}
.conversation-copy:disabled{cursor:wait;opacity:.6}.conversation-copy:focus-visible{outline:2px solid var(--ops-primary);outline-offset:2px}
.conversation-copy strong{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-size:12px;font-weight:600}
.conversation-copy small{font-size:11px;color:var(--ops-muted)}
.chat-welcome{display:flex;flex:1;min-height:0;overflow:auto;align-items:center;justify-content:center;flex-direction:column;padding:24px;text-align:center}
.welcome-icon{display:grid;box-sizing:border-box;width:40px;height:40px;flex:none;place-items:center;color:var(--ops-primary);background:var(--ops-surface-soft);border:1px solid var(--ops-border)}.welcome-icon svg{width:21px;height:21px}
.chat-welcome h2{margin:16px 0 8px;font-size:19px}.chat-welcome p{margin:0;color:var(--ops-muted);font-size:12px;line-height:1.8}
.input-area{flex:none}
.messages-container{min-height:0;flex:1;overflow-y:auto;padding:22px 24px}.message{display:flex;align-items:flex-start;gap:10px;margin-bottom:22px}.message.user{flex-direction:row-reverse}.message-avatar{display:grid;width:34px;height:34px;flex:0 0 34px;place-items:center;color:var(--ops-primary);background:var(--ops-primary-soft);border:1px solid color-mix(in srgb,var(--ops-primary) 22%,var(--ops-border));border-radius:10px}.message.user .message-avatar{color:#fff;background:#27354d;border-color:#27354d}.message-avatar svg{width:18px}.message-body{min-width:0;max-width:min(760px,82%)}.message.user .message-body{display:flex;align-items:flex-end;flex-direction:column}.message-meta{display:flex;align-items:center;gap:8px;margin-bottom:6px;color:var(--ops-muted);font-size:10px}.message-meta strong{color:var(--ops-text);font-size:11px}.message-content{box-sizing:border-box;padding:11px 14px;color:var(--ops-text);background:var(--ops-surface);border:1px solid var(--ops-border-soft);border-radius:4px 12px 12px 12px;font-size:12px;line-height:1.7;box-shadow:0 2px 7px color-mix(in srgb,var(--ops-text) 4%,transparent)}.message.user .message-content{color:#fff;background:#315f95;border-color:#315f95;border-radius:12px 4px 12px 12px;white-space:pre-wrap}.thinking-dots{display:flex;width:58px;gap:5px;padding:14px 16px;background:var(--ops-surface);border:1px solid var(--ops-border-soft);border-radius:4px 12px 12px 12px}.thinking-dots i{width:6px;height:6px;background:var(--ops-primary);border-radius:50%;animation:chat-dot 1.2s infinite ease-in-out}.thinking-dots i:nth-child(2){animation-delay:.15s}.thinking-dots i:nth-child(3){animation-delay:.3s}@keyframes chat-dot{0%,60%,100%{opacity:.25;transform:translateY(0)}30%{opacity:1;transform:translateY(-3px)}}.input-area{padding:12px 16px 10px;background:var(--ops-surface);border-top:1px solid var(--ops-border-soft)}.input-shell{position:relative}.input-shell :deep(.el-textarea__inner){padding:11px 48px 24px 12px;border-radius:8px;box-shadow:0 0 0 1px var(--ops-border) inset!important}.input-shell :deep(.el-input__count){right:47px;bottom:5px;background:transparent;font-size:9px}.send-button{position:absolute;right:10px;bottom:10px;width:31px;height:31px}.input-area>small{display:block;margin-top:6px;color:var(--ops-muted);font-size:9px}.markdown-body :deep(p){margin:0 0 8px}.markdown-body :deep(p:last-child){margin-bottom:0}.markdown-body :deep(h3),.markdown-body :deep(h4),.markdown-body :deep(h5),.markdown-body :deep(h6){margin:12px 0 6px;font-size:13px}.markdown-body :deep(code){padding:1px 4px;color:#315f95;background:var(--ops-primary-soft);border-radius:3px;font:11px ui-monospace,SFMono-Regular,Menlo,monospace}.markdown-body :deep(pre){overflow:auto;margin:8px 0;padding:11px;color:#dbeafe;background:#111827;border-radius:6px;font:10px/1.6 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap}.markdown-body :deep(pre code){padding:0;color:inherit;background:transparent}.markdown-body :deep(ul),.markdown-body :deep(ol){margin:7px 0;padding-left:20px}.markdown-body :deep(blockquote){margin:8px 0;padding:7px 10px;color:var(--ops-muted);background:var(--ops-surface-soft);border-left:3px solid var(--ops-primary)}.markdown-body :deep(hr){margin:12px 0;border:0;border-top:1px solid var(--ops-border)}.markdown-body :deep(a){color:var(--ops-primary)}.markdown-table-wrap{max-width:100%;overflow-x:auto;margin:8px 0}.markdown-body :deep(table){width:100%;border-collapse:collapse}.markdown-body :deep(th),.markdown-body :deep(td){padding:6px 8px;text-align:left;border:1px solid var(--ops-border);font-size:10px}.markdown-body :deep(th){background:var(--ops-surface-soft);font-weight:650}
.message-content{overflow-wrap:anywhere}.markdown-body :deep(.markdown-table-wrap){max-width:100%;overflow-x:auto;margin:8px 0}
@media(max-width:600px){.chat-header{padding:10px 12px}.messages-container{padding:16px 12px}.message-body{max-width:85%}.chat-welcome{justify-content:flex-start;padding:20px}}
</style>

<style scoped>
.chat-model-notice{padding:8px 16px;border-bottom:1px solid var(--ops-border)}.chat-model-notice small{font-size:11px;color:var(--ops-muted)}
</style>

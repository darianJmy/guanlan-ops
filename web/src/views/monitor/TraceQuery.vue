<template>
  <div class="trace-query">
    <div class="trace-query-panel">
      <div class="query-header"><div class="service-title"><span class="service-icon"><img :src="jaegerLogo" alt="Jaeger" /></span><strong :title="source.name">{{ source.name }}</strong></div><el-radio-group class="query-type-switch" v-model="queryType" :disabled="loading" @change="resetResults"><el-radio-button value="search">Search</el-radio-button><el-radio-button value="traceID">Trace ID</el-radio-button></el-radio-group></div>
      <el-form v-if="queryType === 'search'" label-position="left" label-width="90px" class="search-form" :disabled="loading">
        <el-form-item label="服务名称"><el-select v-model="service" filterable allow-create default-first-option :loading="metadataLoading" placeholder="选择或输入服务名称" @change="changeService"><el-option v-for="name in services" :key="name" :value="name" :label="name" /></el-select></el-form-item>
        <el-form-item label="操作名称"><el-select v-model="operation" filterable allow-create default-first-option clearable placeholder="全部操作"><el-option v-for="name in operations" :key="name" :value="name" :label="name" /></el-select></el-form-item>
        <el-form-item label="时间范围" class="range-field"><el-date-picker v-model="range" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" :shortcuts="shortcuts" /></el-form-item>
        <el-form-item label="标签"><el-input v-model="tags" placeholder="http.status_code=200 error=true" /></el-form-item>
        <el-form-item label="最小耗时"><el-input v-model="minDuration" placeholder="例如 100ms、1.2s" /></el-form-item>
        <el-form-item label="最大耗时"><el-input v-model="maxDuration" placeholder="例如 500ms、2s" /></el-form-item>
        <el-form-item label="返回条数"><el-select v-model="limit"><el-option v-for="n in [20,50,100]" :key="n" :value="n" :label="`${n} 条`" /></el-select></el-form-item>
      </el-form>
      <el-alert v-if="metadataError && queryType === 'search'" :title="metadataError" type="warning" :closable="false"><el-button text @click="loadServices">重试加载服务</el-button></el-alert>
      <div class="toolbar"><el-input v-if="queryType === 'traceID'" v-model="traceID" placeholder="输入 16 或 32 位 Trace ID" aria-label="Trace ID" :disabled="loading" @keyup.enter="execute" /><el-button type="primary" :loading="loading" @click="execute">{{ queryType === 'search' ? '搜索链路' : '查询链路' }}</el-button></div>
    </div>
    <div class="trace-results-panel" v-loading="loading">
      <el-alert v-if="error" :title="error" type="error" :closable="false" />
      <el-empty v-if="executed && !traces.length" description="未找到链路" />
      <el-table v-if="executed && traces.length && queryType === 'search'" :data="traces" @row-click="selectTrace" class="trace-list" max-height="480">
        <el-table-column prop="traceID" label="Trace ID" min-width="250" />
        <el-table-column label="Trace Name" min-width="280"><template #default="{ row }">{{ rootName(row) }}</template></el-table-column>
        <el-table-column label="Start Time" min-width="190"><template #default="{ row }">{{ new Date(start(row)/1000).toLocaleString('zh-CN', { hour12: false }) }}</template></el-table-column>
        <el-table-column label="Duration" width="130"><template #default="{ row }">{{ (duration(row)/1000).toFixed(2) }} ms</template></el-table-column>
      </el-table>
      <el-empty v-if="!executed && !loading && !error" description="选择服务和时间范围，搜索调用链路" />
      <section v-for="trace in visibleTraces" :key="trace.traceID">
        <p>{{ trace.traceID }} · {{ trace.spans.length }} 个 Span · {{ (duration(trace)/1000).toFixed(2) }} ms</p>
        <el-table :data="sortedSpans(trace)" row-key="spanID" :expand-row-keys="expandedSpans[trace.traceID] || []" class="span-table" max-height="650" @row-click="row => toggleSpan(trace, row)" @expand-change="(_row, rows) => expandedSpans[trace.traceID] = rows.map(item => item.spanID)">
          <el-table-column type="expand"><template #default="{ row }"><SpanDetail :span="row" :process="trace.processes?.[row.processID] || {}" :trace-start="start(trace)" :trace-id="trace.traceID" /></template></el-table-column>
          <el-table-column label="服务 / 操作" min-width="240"><template #default="{ row }"><span :style="{ paddingLeft: `${depth(trace,row)*16}px` }">{{ trace.processes?.[row.processID]?.serviceName }}: {{ displayOperation(row) }}</span></template></el-table-column>
          <el-table-column label="耗时" width="130"><template #default="{ row }">{{ (row.duration / 1000).toFixed(2) }} ms</template></el-table-column>
          <el-table-column label="调用时间线" min-width="280"><template #default="{ row }"><div class="timeline"><div class="bar" :style="{ marginLeft: `${(row.startTime-start(trace))/duration(trace)*100}%`, width: `${Math.max(0.5,row.duration/duration(trace)*100)}%` }" /></div></template></el-table-column>
        </el-table>
      </section>
    </div>
  </div>
</template>
<script setup>
import { computed, ref, onMounted, onBeforeUnmount } from 'vue'
import request from '../../api/request'
import SpanDetail from '../../components/explore/SpanDetail.vue'
import jaegerLogo from '../../assets/jaeger-logo.svg'
const props=defineProps({source:{type:Object,required:true}})
const queryType=ref('search'), service=ref(''), operation=ref(''), tags=ref(''), minDuration=ref(''), maxDuration=ref(''), limit=ref(20)
const range=ref([new Date(Date.now()-3600000),new Date()]), services=ref([]), operations=ref([]), metadataLoading=ref(false), metadataError=ref(''), selectedTrace=ref(null)
const shortcuts=[15,60,360,1440].map(minutes=>({text:minutes<60?`最近 ${minutes} 分钟`:`最近 ${minutes/60} 小时`,value:()=>[new Date(Date.now()-minutes*60000),new Date()]}))
const visibleTraces=computed(()=>queryType.value==='traceID'?traces.value:selectedTrace.value?[selectedTrace.value]:[])
function resetResults(){ expandedSpans.value={}; traces.value=[]; selectedTrace.value=null; executed.value=false; error.value='' }
function selectTrace(trace){selectedTrace.value=trace}
function displayOperation(span){
 const name=span.operationName || ''
 const method=/^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|CONNECT|TRACE)(?:\s+|$)/i
 if(!method.test(name))return name
 const route=span.tags?.find(tag=>tag.key==='http.route')?.value
 return typeof route==='string' && route ? route : name.replace(method,'').trim() || name
}
function rootName(trace){const root=trace.spans.find(item=>!item.references?.some(ref=>ref.refType==='CHILD_OF')) || trace.spans[0];return root?`${trace.processes?.[root.processID]?.serviceName || ''}: ${displayOperation(root)}`:'—'}
const metadataController=new AbortController()
let operationRequest=0
async function loadServices(){
 metadataLoading.value=true;metadataError.value=''
 try{const response=await request({url:'/query/traces/metadata',params:{datasource_id:props.source.id},signal:metadataController.signal});services.value=response.data;if(!service.value && services.value.length){service.value=services.value[0];await changeService()}}
 catch(e){if(e.code!=='ERR_CANCELED')metadataError.value=e.response?.data?.message||e.message}
 finally{metadataLoading.value=false}
}
async function changeService(){
 const current=++operationRequest;operation.value='';operations.value=[];metadataError.value=''
 if(!service.value)return
 try{const response=await request({url:'/query/traces/metadata',params:{datasource_id:props.source.id,service:service.value},signal:metadataController.signal});if(current===operationRequest)operations.value=response.data}
 catch(e){if(current===operationRequest && e.code!=='ERR_CANCELED')metadataError.value='操作列表加载失败，可手动输入操作名称'}
}
function parseTags(text){
 if(!text.trim())return {}
 if(text.trim().startsWith('{')){const parsed=JSON.parse(text);if(!parsed || Array.isArray(parsed) || typeof parsed!=='object')throw new Error('标签必须为 JSON 对象');return Object.fromEntries(Object.entries(parsed).map(([key,value])=>[key,String(value)]))}
 const result={};let rest=text.trim()
 while(rest){const match=/^([^\s=]+)=("(?:[^"\\]|\\.)*"|[^\s]+)(?:\s+|$)/.exec(rest);if(!match)throw new Error('标签格式应为 key=value，含空格的值请加双引号');result[match[1]]=match[2].startsWith('"')?JSON.parse(match[2]):match[2];rest=rest.slice(match[0].length)}
 return result
}
const traceID=ref(''),loading=ref(false),error=ref(''),traces=ref([]),executed=ref(false),expandedSpans=ref({})
const sortedSpans=trace=>[...trace.spans].sort((a,b)=>a.startTime-b.startTime)
function depth(trace,row){
 const spans=new Map(trace.spans.map(item=>[item.spanID,item])), seen=new Set([row.spanID]);let level=0,current=row
 while(level<20){const parent=current.references?.find(item=>item.refType==='CHILD_OF')?.spanID;if(!parent||seen.has(parent)||!spans.has(parent))break;seen.add(parent);current=spans.get(parent);level++}
 return level
}
const start=trace=>trace.spans.length?Math.min(...trace.spans.map(item=>item.startTime)):0
const duration=trace=>trace.spans.length ? Math.max(1,Math.max(...trace.spans.map(item=>item.startTime+item.duration))-start(trace)) : 1
function toggleSpan(trace,row){
 const keys=expandedSpans.value[trace.traceID] || []
 expandedSpans.value[trace.traceID]=keys.includes(row.spanID)?keys.filter(key=>key!==row.spanID):[...keys,row.spanID]
}
let controller
async function execute(){
 if(loading.value)return
 resetResults()
 let data={datasource_id:props.source.id}
 if(queryType.value==='traceID'){
  if(!/^(?:[a-f\d]{16}|[a-f\d]{32})$/i.test(traceID.value.trim())){error.value='请输入有效的 Trace ID';return}
  data.trace_id=traceID.value.trim()
 }else{
  const [from,to]=range.value||[]
  if(!service.value.trim() || !from || !to || to<=from || to-from>31*86400000){error.value='请选择服务和有效时间范围，最长 31 天';return}
  try{data={...data,service:service.value,operation:operation.value,tags:parseTags(tags.value),start:from.toISOString(),end:to.toISOString(),min_duration:minDuration.value.trim(),max_duration:maxDuration.value.trim(),limit:limit.value}}
  catch(e){error.value=e.message;return}
 }
 loading.value=true;controller=new AbortController()
 try{const response=await request({url:'/query/traces',method:'post',timeout:35000,signal:controller.signal,data});traces.value=response.data;executed.value=true}
 catch(e){if(e.code!=='ERR_CANCELED')error.value=e.response?.data?.message||e.message}
 finally{loading.value=false}
}
onMounted(loadServices)
onBeforeUnmount(()=>{controller?.abort();metadataController.abort()})
</script>
<style scoped>
.trace-query{display:flex;flex-direction:column;gap:16px;min-width:0}.trace-query-panel,.trace-results-panel{display:flex;flex-direction:column;gap:16px;min-width:0;padding:16px;background:var(--ops-surface);border:1px solid var(--ops-border);box-sizing:border-box}.trace-results-panel{min-height:200px}.toolbar{display:flex;gap:12px;justify-content:flex-end;width:560px;max-width:100%;align-self:flex-start}.timeline{height:16px;background:var(--ops-surface-soft);overflow:hidden}.bar{height:100%;background:var(--ops-primary);border-radius:3px}pre{white-space:pre-wrap;overflow-wrap:anywhere}
.query-header{display:flex;align-items:center;justify-content:space-between;gap:12px;min-width:0;flex-wrap:wrap}.query-type-switch{margin-left:auto;max-width:100%}.query-header strong{overflow-wrap:anywhere}.search-form{display:grid;grid-template-columns:minmax(0,1fr);gap:0 16px;width:560px;max-width:100%;align-self:flex-start}.search-form :deep(.el-select){width:100%}.range-field{grid-column:auto}.search-form :deep(.el-date-editor){width:100%;min-width:0;box-sizing:border-box}.search-form :deep(.el-range-input){min-width:0}.trace-list :deep(.el-table__row){cursor:pointer}
@media(max-width:520px){.search-form{grid-template-columns:minmax(0,1fr)}.range-field{grid-column:auto}.toolbar{flex-wrap:wrap}}
.service-title { display: flex; align-items: center; gap: 8px; min-width: 0; max-width: 100%; }
.service-icon { display: grid; place-items: center; flex: 0 0 28px; height: 28px; background: var(--ops-surface-soft); border: 1px solid var(--ops-border-soft); border-radius: 4px; }
.service-icon img { display: block; width: 18px; height: 20px; object-fit: contain; }
.service-title strong { min-width: 0; max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 14px; font-weight: 600; }
.span-table :deep(.el-table__row){cursor:pointer}
</style>

<template>
  <section class="workbench">
    <header class="workbench-toolbar">
      <div class="time-tools">
        <el-select v-model="duration" aria-label="时间范围"><el-option v-for="item in durations" :key="item.value" :value="item.value" :label="item.label" /><el-option :value="0" label="自定义时间" /></el-select>
        <el-date-picker v-if="!duration" v-model="customRange" type="datetimerange" start-placeholder="开始时间" end-placeholder="结束时间" />
        <el-select v-model="refresh" aria-label="自动刷新间隔"><el-option :value="0" label="自动刷新关闭" /><el-option v-for="seconds in [10,30,60]" :key="seconds" :value="seconds" :label="`${seconds} 秒刷新`" /></el-select>
      </div>
      <div class="actions"><el-button @click="historyOpen=true">历史 / 收藏</el-button><el-button @click="inspectorOpen=true">查询详情</el-button><el-button v-if="loading" @click="cancel">取消</el-button><el-button type="primary" :loading="loading" @click="run">执行查询</el-button></div>
    </header>
    <div class="options-row">
      <span class="language">{{ languageLabel }}</span>
      <template v-if="metrics"><el-radio-group v-model="queryType" size="small"><el-radio-button value="range">范围查询</el-radio-button><el-radio-button value="instant">即时值</el-radio-button></el-radio-group><span>步长</span><el-select v-model="step" size="small" aria-label="查询步长"><el-option :value="0" label="自动" /><el-option v-for="n in [1,10,30,60,300,900,3600]" :key="n" :value="n" :label="`${n} 秒`" /></el-select></template>
      <el-select v-else v-model="limit" size="small" aria-label="日志返回条数"><el-option v-for="n in [100,500,1000,5000]" :key="n" :value="n" :label="`最多 ${n} 条`" /></el-select>
      <span class="shortcut">Ctrl / ⌘ + Enter 执行</span>
    </div>
    <article v-for="(item,index) in queries" :key="item.id" class="query-card" :class="{disabled:!item.enabled}">
      <div class="query-card-header"><span class="query-letter">{{ item.id }}</span><span>{{ source.name }}</span><span class="editor-mode">代码</span><el-switch v-model="item.enabled" size="small" :aria-label="`启用查询 ${item.id}`" /><el-button text size="small" @click="favorite(item)">收藏</el-button><el-button v-if="queries.length>1" text size="small" @click="queries.splice(index,1)">删除</el-button></div>
      <QueryEditor v-model="item.text" :language="metrics?'promql':source.type==='loki'?'logql':'plaintext'" @execute="run" />
      <div class="query-card-footer"><el-button text size="small" @click="item.text=example">填入示例</el-button><el-button v-if="metrics" text size="small" @click="browseMetrics(item)">浏览指标</el-button><span>{{ item.error || (item.elapsed ? `${item.elapsed} ms` : '准备就绪') }}</span></div>
    </article>
    <div v-if="metrics" class="below-editor"><el-button v-if="metrics" :disabled="queries.length>=6" @click="addQuery">＋ 添加查询</el-button></div>
    <el-alert v-if="error" :title="error" type="error" :closable="false" show-icon />
    <el-alert v-if="limitReached" title="已达到日志条数上限。趋势和本地筛选仅基于已返回的数据，可缩小时间范围继续查询。" type="warning" :closable="false" />
    <section v-if="executed" class="results">
      <div class="results-heading"><strong>{{ metrics ? '查询结果' : '日志' }}</strong><span>{{ metrics ? `${series.length} 条序列` : `${entries.length} 条返回日志` }} · {{ elapsed }} ms</span><span v-if="executedRange">{{ executedRange[0].toLocaleString() }} — {{ executedRange[1].toLocaleString() }}</span></div>
      <template v-if="metrics">
        <el-tabs v-model="resultTab"><el-tab-pane label="图表" name="graph" /><el-tab-pane label="表格" name="table" /></el-tabs>
        <v-chart v-if="resultTab==='graph' && series.length" :option="chartOptions" autoresize class="metric-chart" />
        <el-table v-else-if="resultTab==='table'" :data="metricRows" max-height="500" size="small"><el-table-column prop="name" label="序列 / 标签" min-width="300" show-overflow-tooltip /><el-table-column prop="latest" label="最新值" width="150" /><el-table-column prop="min" label="最小值" width="130" /><el-table-column prop="max" label="最大值" width="130" /><el-table-column prop="count" label="采样点" width="90" /></el-table>
        <el-empty v-if="!series.length" description="没有匹配的指标数据" />
        <p v-if="series.length && resultTab==='graph'" class="hint">点击图例隐藏 / 显示序列，滚轮或拖动底部滑块缩放。</p>
      </template>
      <template v-else>
        <div class="histogram-title">当前返回日志分布 <span>非全量日志统计 · 点击柱形调整查询时间范围</span></div>
        <v-chart v-if="entries.length" :option="histogram" autoresize class="histogram" @click="selectBucket" />
        <div class="log-tools"><el-input v-model="keyword" clearable placeholder="在返回日志中搜索并高亮" aria-label="搜索返回日志" /><el-select v-model="level" clearable placeholder="全部级别" aria-label="日志级别"><el-option v-for="value in levels" :key="value" :value="value" :label="value.toUpperCase()" /></el-select><el-switch v-model="wrap" active-text="换行" /><el-button @click="ascending=!ascending">{{ ascending?'最早优先':'最新优先' }}</el-button><span>{{ filteredEntries.length }} 条</span></div>
        <div class="log-lines" :class="{nowrap:!wrap}">
          <article v-for="(entry,index) in pageEntries" :key="`${entry.timestamp}-${index}`" class="log-row" :class="`level-${logLevel(entry)}`">
            <button class="log-content" @click="selectedEntry=entry;detailOpen=true"><time>{{ timestamp(entry.timestamp) }}</time><span class="severity">{{ logLevel(entry).toUpperCase() }}</span><span class="line-text"><template v-for="(part,i) in highlightParts(entry.line,keyword)" :key="i"><mark v-if="part.match">{{ part.text }}</mark><template v-else>{{ part.text }}</template></template></span></button>
          </article>
          <el-empty v-if="!filteredEntries.length" description="当前筛选条件下没有日志" />
        </div>
        <el-pagination v-if="filteredEntries.length>100" v-model:current-page="page" :page-size="100" :total="filteredEntries.length" layout="prev,pager,next,total" />
      </template>
    </section>
    <el-empty v-else-if="!loading && !error" description="输入查询，开始探索数据" />
    <el-drawer v-model="detailOpen" title="日志详情" size="min(720px,95vw)"><template v-if="selectedEntry"><p>{{ timestamp(selectedEntry.timestamp) }}</p><pre class="log-detail">{{ selectedEntry.line }}</pre><el-button v-if="source.type!=='clickhouse'" @click="applyFilter('',selectedEntry.line)">按正文过滤</el-button><h4>标签</h4><div v-for="(value,key) in selectedEntry.labels" :key="key" class="field-row"><code>{{ key }}</code><span>{{ value }}</span><el-button v-if="source.type==='loki'" text @click="applyFilter(key,value)">加入查询</el-button></div><h4>元数据 / 字段</h4><pre class="log-detail">{{ JSON.stringify(selectedEntry.fields || selectedEntry.metadata || {},null,2) }}</pre></template></el-drawer>
    <el-drawer v-model="historyOpen" title="本机查询历史与收藏" size="min(520px,95vw)"><p class="hint">按当前账号、数据源和查询模式保存在本浏览器，最多 30 条。</p><el-tabs v-model="historyTab"><el-tab-pane label="历史" name="history" /><el-tab-pane label="收藏" name="favorites" /></el-tabs><div v-for="(item,index) in stored[historyTab]" :key="index" class="saved-item"><button @click="restore(item)"><code>{{ item.text }}</code><small>{{ new Date(item.time).toLocaleString() }}</small></button><el-button text @click="stored[historyTab].splice(index,1);persist()">删除</el-button></div><el-empty v-if="!stored[historyTab].length" description="暂无记录" /></el-drawer>
    <el-drawer v-model="inspectorOpen" title="查询详情" size="min(700px,95vw)"><p class="hint">本次查询的参数、耗时与状态，不包含认证信息。</p><pre class="log-detail">{{ JSON.stringify(inspector,null,2) }}</pre></el-drawer>
    <el-dialog v-model="metricsOpen" title="指标浏览器" width="min(650px,95vw)"><el-input v-model="metricSearch" clearable placeholder="筛选指标名称" /><div v-loading="metadataLoading" class="metric-browser"><button v-for="name in matchingMetrics" :key="name" @click="metricTarget.text=name;metricsOpen=false">{{ name }}</button><p v-if="metadataError">{{ metadataError }}</p></div><p class="hint">最多展示 200 个匹配项，选择后填入编辑器。</p></el-dialog>
  </section>
</template>
<script setup>
import { computed,onBeforeUnmount,onMounted,ref,watch } from 'vue'
import { ElMessage } from 'element-plus'
import { use } from 'echarts/core'
import { LineChart,BarChart } from 'echarts/charts'
import { GridComponent,TooltipComponent,LegendComponent,DataZoomComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import VChart from 'vue-echarts'
import QueryEditor from './QueryEditor.vue'
import request from '../../api/request'
import { useUserStore } from '../../stores/user'
import { useThemeColors } from '../../theme/useThemeColors'
import { durations,queryRange,queryStep,highlightParts,logLevel,appendLogFilter,metricSeries } from './query-utils.mjs'
use([LineChart,BarChart,GridComponent,TooltipComponent,LegendComponent,DataZoomComponent,CanvasRenderer])
const props=defineProps({source:{type:Object,required:true},mode:{type:String,required:true},initial:{type:Object,default:null}})
const emit=defineEmits(['state','time-change'])
const theme=useThemeColors(), user=useUserStore(), metrics=computed(()=>props.mode==='metrics')
const duration=ref(props.initial?.duration ?? 3600),customRange=ref(props.initial?.customRange?.map(value=>new Date(value)) || []),refresh=ref(0),step=ref(0),limit=ref(500),queryType=ref(props.initial?.queryType || 'range')
let nextID=1
const queries=ref(props.initial?.queries?.map((item,index)=>({id:String.fromCharCode(65+index),text:item.text,enabled:item.enabled!==false,error:'',elapsed:0})) || [{id:'A',text:'',enabled:true,error:'',elapsed:0}])
nextID=queries.value.length
const loading=ref(false),error=ref(''),executed=ref(false),elapsed=ref(0),executedRange=ref(null),series=ref([]),entries=ref([]),limitReached=ref(false),resultTab=ref('graph'),inspector=ref({})
const keyword=ref(''),level=ref(''),wrap=ref(true),ascending=ref(false),page=ref(1),selectedEntry=ref(null),detailOpen=ref(false),historyOpen=ref(false),historyTab=ref('history'),inspectorOpen=ref(false)
const languageLabel=computed(()=>metrics.value?'PromQL':props.source.type==='loki'?'LogQL':props.source.type==='elasticsearch'?'Elasticsearch':'日志关键词')
const example=computed(()=>metrics.value?'sum by (job) (rate(http_requests_total[5m]))':props.source.type==='loki'?'{app="api"} |= "error"':props.source.type==='elasticsearch'?'error AND service.name:api':'error')
const storageKey=`guanlan:explore:${user.userInfo?.id || 'anonymous'}:${props.source.id}:${props.mode}`
const stored=ref({history:[],favorites:[]})
try { const saved=JSON.parse(localStorage.getItem(storageKey)||'{}'); for(const key of ['history','favorites']) stored.value[key]=(Array.isArray(saved[key])?saved[key]:[]).filter(item=>typeof item.text==='string').slice(0,30) } catch {}
function persist(){try{localStorage.setItem(storageKey,JSON.stringify(stored.value))}catch{ElMessage.warning('浏览器存储不可用，记录仅保留到本次页面关闭')}}
function saveQuery(item,target){stored.value[target]=[{text:item.text,time:Date.now(),queryType:queryType.value},...stored.value[target].filter(value=>value.text!==item.text)].slice(0,30);persist()}
function favorite(item){if(!item.text.trim())return;saveQuery(item,'favorites');ElMessage.success('已收藏')}
function restore(item){queries.value[0].text=item.text;queryType.value=item.queryType||'range';historyOpen.value=false}
function addQuery(){queries.value.push({id:String.fromCharCode(65+nextID++),text:'',enabled:true,error:'',elapsed:0})}
function snapshot(){return {queries:queries.value.map(({text,enabled})=>({text,enabled})),duration:duration.value,customRange:customRange.value,queryType:queryType.value}}
watch([queries,duration,customRange,queryType],()=>emit('state',snapshot()),{deep:true})
watch([duration,customRange],()=>emit('time-change',{duration:duration.value,customRange:customRange.value}),{deep:true})
function setTime(value){duration.value=value.duration;customRange.value=(value.customRange||[]).map(item=>new Date(item))}
defineExpose({snapshot,setTime})
let controller,timer,disposed=false
function cancel(){controller?.abort();refresh.value=0}
async function run(){
 if(loading.value)return
 const active=queries.value.filter(item=>item.enabled&&item.text.trim())
 if(!active.length){ElMessage.warning('请输入并启用至少一条查询');return}
 let range,resolution
 try{range=queryRange(duration.value,customRange.value);resolution=queryStep(range,metrics.value?step.value:0)}catch(e){error.value=e.message;return}
 loading.value=true;error.value='';executed.value=false;series.value=[];entries.value=[];limitReached.value=false;executedRange.value=range;controller=new AbortController()
 const started=performance.now();const details=[]
 for(const item of queries.value){item.error='';item.elapsed=0}
 const outcomes=await Promise.all(active.map(async item=>{
  const text=item.text,at=performance.now(),endpoint=metrics.value?(queryType.value==='range'?'/prometheus/query_range':'/prometheus/query'):'/query/logs'
  const body={datasource_id:props.source.id,query:text,...(metrics.value?(queryType.value==='range'?{start:String(range[0].getTime()/1000),end:String(range[1].getTime()/1000),step:String(resolution)}:{time:String(range[1].getTime()/1000)}):{start:range[0].toISOString(),end:range[1].toISOString(),limit:limit.value})}
  try{const response=await request({url:endpoint,method:'post',data:body,timeout:35000,signal:controller.signal});item.elapsed=Math.round(performance.now()-at);details.push({query:item.id,endpoint,parameters:body,elapsed_ms:item.elapsed,status:'success'});saveQuery({text},'history');return {id:item.id,data:response.data}}
  catch(e){if(e.code!=='ERR_CANCELED')item.error=e.response?.data?.message||e.message;details.push({query:item.id,endpoint,parameters:body,status:e.code==='ERR_CANCELED'?'cancelled':'error',error:item.error});return null}
 }))
 if(disposed)return
 elapsed.value=Math.round(performance.now()-started);inspector.value={datasource:props.source.name,elapsed_ms:elapsed.value,queries:details};loading.value=false
 if(controller.signal.aborted){error.value='查询已取消';return}
 const successes=outcomes.filter(Boolean)
 if(metrics.value)series.value=successes.flatMap(item=>metricSeries(item.data,item.id))
 else {entries.value=successes.flatMap(item=>item.data.entries||[]);limitReached.value=successes.some(item=>item.data.limit_reached)}
 error.value=active.filter(item=>item.error).map(item=>`${item.id}: ${item.error}`).join('；')
 executed.value=successes.length>0;page.value=1
}
watch(refresh,value=>{clearInterval(timer);if(value)timer=setInterval(()=>{if(!document.hidden&&!loading.value)run()},value*1000)})
onBeforeUnmount(()=>{disposed=true;clearInterval(timer);controller?.abort()})
onMounted(()=>emit('state',snapshot()))
const metricRows=computed(()=>series.value.map(item=>{const values=item.data.map(point=>Number(point[1])).filter(Number.isFinite);return {name:item.name,latest:item.data.at(-1)?.[1]??'-',min:values.length?Math.min(...values):'-',max:values.length?Math.max(...values):'-',count:item.data.length}}))
const chartOptions=computed(()=>({backgroundColor:'transparent',textStyle:{color:theme.value.text},tooltip:{trigger:'axis',renderMode:'richText'},legend:{type:'scroll',bottom:0,textStyle:{color:theme.value.muted}},grid:{left:60,right:22,top:20,bottom:85},xAxis:{type:'time',axisLabel:{color:theme.value.muted}},yAxis:{type:'value',scale:true,axisLabel:{color:theme.value.muted},splitLine:{lineStyle:{color:theme.value.border}}},dataZoom:[{type:'inside'},{type:'slider',bottom:32,height:18}],series:series.value.map(item=>({name:item.name,type:'line',showSymbol:item.data.length===1,symbolSize:7,connectNulls:false,data:item.data.map(([time,value])=>[time*1000,Number.isFinite(Number(value))?Number(value):null])}))}))
const levels=computed(()=>[...new Set(entries.value.map(logLevel))].sort())
const filteredEntries=computed(()=>entries.value.filter(item=>(!level.value||logLevel(item)===level.value)&&(!keyword.value||item.line.toLocaleLowerCase().includes(keyword.value.toLocaleLowerCase()))).sort((a,b)=>{const x=BigInt(a.timestamp),y=BigInt(b.timestamp);return (x===y?0:x<y?-1:1)*(ascending.value?1:-1)}))
watch([keyword,level,ascending],()=>page.value=1)
const pageEntries=computed(()=>filteredEntries.value.slice((page.value-1)*100,page.value*100))
const timestamp=value=>new Date(Number(BigInt(value)/1000000n)).toLocaleString('zh-CN',{hour12:false})+'.'+String(Number(BigInt(value)/1000000n)%1000).padStart(3,'0')
const buckets=computed(()=>{if(!executedRange.value)return [];const [start,end]=executedRange.value.map(date=>date.getTime()),width=(end-start)/40;const result=Array.from({length:40},(_,i)=>({start:start+i*width,end:start+(i+1)*width,count:0}));for(const entry of entries.value){const i=Math.floor((Number(BigInt(entry.timestamp)/1000000n)-start)/width);if(i>=0&&i<40)result[i].count++}return result})
const histogram=computed(()=>({tooltip:{trigger:'axis',renderMode:'richText'},grid:{left:42,right:16,top:8,bottom:25},xAxis:{type:'category',data:buckets.value.map(item=>new Date(item.start).toLocaleTimeString()),axisLabel:{color:theme.value.muted}},yAxis:{type:'value',minInterval:1,axisLabel:{color:theme.value.muted},splitLine:{lineStyle:{color:theme.value.border}}},series:[{type:'bar',data:buckets.value.map(item=>item.count),itemStyle:{color:theme.value.primary}}]}))
function selectBucket(event){const bucket=buckets.value[event.dataIndex];if(bucket){duration.value=0;customRange.value=[new Date(bucket.start),new Date(bucket.end)];ElMessage.info('已调整时间范围，点击执行查询加载日志')}}
function applyFilter(field,value){try{queries.value[0].text=appendLogFilter(queries.value[0].text,props.source.type,field,value);detailOpen.value=false;ElMessage.success('已加入查询，点击执行生效')}catch(e){ElMessage.warning(e.message)}}
const metricsOpen=ref(false),metadataLoading=ref(false),metadataError=ref(''),metricSearch=ref(''),metricNames=ref([]),metricTarget=ref(null)
const matchingMetrics=computed(()=>metricNames.value.filter(name=>name.toLowerCase().includes(metricSearch.value.toLowerCase())).slice(0,200))
let metadataController
async function browseMetrics(item){metricTarget.value=item;metricsOpen.value=true;if(metricNames.value.length)return;metadataLoading.value=true;metadataError.value='';metadataController=new AbortController();try{const response=await request({url:'/prometheus/metadata',params:{datasource_id:props.source.id,target:'metrics'},signal:metadataController.signal,timeout:35000});metricNames.value=response.data||[]}catch(e){if(e.code!=='ERR_CANCELED')metadataError.value=e.message}finally{metadataLoading.value=false}}
onBeforeUnmount(()=>metadataController?.abort())
</script>
<style scoped>
.workbench{display:flex;flex-direction:column;gap:12px;min-width:0}.workbench-toolbar,.time-tools,.actions,.options-row,.query-card-header,.query-card-footer,.below-editor,.results-heading,.log-tools{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.workbench-toolbar{justify-content:space-between;padding:12px;background:var(--ops-surface);border:1px solid var(--ops-border);border-radius:6px}.time-tools>.el-select{width:155px}.actions{margin-left:auto}.options-row{font-size:12px;color:var(--ops-muted)}.options-row .el-select{width:130px}.language{font-weight:600;color:var(--ops-text)}.shortcut{margin-left:auto}.query-card{border:1px solid var(--ops-border);border-radius:6px;overflow:hidden;background:var(--ops-surface)}.query-card.disabled{opacity:.55}.query-card-header{padding:7px 12px;background:var(--ops-surface-soft);font-size:12px}.query-letter{background:var(--ops-primary);color:white;border-radius:3px;padding:3px 7px;font-weight:700}.editor-mode{margin-left:auto;color:var(--ops-muted)}.query-card-footer{padding:3px 12px;border-top:1px solid var(--ops-border-soft)}.query-card-footer>span{margin-left:auto;color:var(--ops-muted);font-size:12px}.below-editor>span,.hint{font-size:12px;color:var(--ops-muted)}.results{border:1px solid var(--ops-border);border-radius:6px;padding:14px;min-width:0}.results-heading{font-size:12px;color:var(--ops-muted);margin-bottom:12px}.results-heading strong{font-size:14px;color:var(--ops-text)}.metric-chart{height:390px}.histogram{height:135px}.histogram-title{font-size:13px}.histogram-title span{font-size:11px;color:var(--ops-muted);margin-left:8px}.log-tools{margin:14px 0;font-size:12px}.log-tools>.el-input{width:260px;max-width:100%}.log-tools>.el-select{width:130px}.log-lines{max-height:550px;overflow:auto;border:1px solid var(--ops-border-soft);font-family:Consolas,monospace}.log-row{border-bottom:1px solid var(--ops-border-soft);border-left:3px solid var(--ops-muted)}.log-row.level-error,.log-row.level-fatal{border-left-color:var(--ops-danger)}.log-row.level-warn{border-left-color:#d99b30}.log-row.level-info{border-left-color:var(--ops-primary)}.log-content{display:flex;gap:12px;text-align:left;width:100%;border:0;background:transparent;color:var(--ops-text);padding:8px;cursor:pointer;font:inherit;font-size:12px}.log-content:hover{background:var(--ops-surface-soft)}.log-content time{flex:0 0 175px;color:var(--ops-muted)}.severity{flex:0 0 55px;font-size:10px;color:var(--ops-muted)}.line-text{white-space:pre-wrap;overflow-wrap:anywhere;min-width:0}.nowrap .line-text{white-space:pre}.nowrap .log-content{width:max-content;min-width:100%}mark{background:#ffdc75;color:#292100}.log-detail{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;background:var(--ops-surface-soft);padding:14px}.field-row{display:flex;gap:12px;align-items:center;border-bottom:1px solid var(--ops-border);padding:8px 0;overflow-wrap:anywhere}.field-row span{flex:1;min-width:0}.saved-item{display:flex;border-bottom:1px solid var(--ops-border);padding:12px 0}.saved-item>button{flex:1;min-width:0;border:0;background:transparent;color:var(--ops-text);text-align:left;cursor:pointer}.saved-item code{display:block;white-space:pre-wrap;overflow-wrap:anywhere}.saved-item small{display:block;color:var(--ops-muted);margin-top:6px}.metric-browser{display:flex;flex-direction:column;max-height:400px;overflow:auto;margin-top:12px}.metric-browser button{border:0;border-bottom:1px solid var(--ops-border-soft);text-align:left;background:transparent;padding:8px;color:var(--ops-text);cursor:pointer}.metric-browser button:hover{background:var(--ops-surface-soft)}@media(max-width:700px){.actions{margin-left:0}.log-content{flex-wrap:wrap}.line-text{flex-basis:100%}.time-tools{width:100%}.time-tools :deep(.el-date-editor){max-width:100%}.shortcut{display:none}}
/* Controls adapt to each pane's width, including side-by-side queries. */
.workbench { container-type: inline-size; }
.workbench-toolbar { padding: 0 0 12px; border: 0; border-bottom: 1px solid var(--ops-border-soft); border-radius: 0; background: transparent; gap: 12px; }
.time-tools { flex: 1 1 320px; min-width: 0; gap: 8px; }
.actions { gap: 8px; flex-shrink: 0; }
.actions :deep(.el-button + .el-button), .query-card-header :deep(.el-button + .el-button) { margin-left: 0; }
.time-tools :deep(.el-date-editor) { flex: 1 1 100%; width: 100%; min-width: 0; box-sizing: border-box; }
.time-tools :deep(.el-range-input) { min-width: 0; }
.options-row { min-height: 32px; gap: 12px; }
.query-card-header { flex-wrap: nowrap; min-height: 36px; }
.query-card-header > span:nth-child(2) { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
.query-card-header .el-switch, .query-card-header .el-button { flex-shrink: 0; }
.query-card-footer > span { overflow-wrap: anywhere; min-width: 0; }
.results-heading { row-gap: 6px; }
.results-heading > span:last-child { overflow-wrap: anywhere; }
@container (max-width: 720px) {
  .workbench-toolbar { flex-direction: column; align-items: stretch; }
  .time-tools { flex: none; width: 100%; }
  .time-tools > .el-select { flex: 1; width: auto; min-width: 130px; }
  .actions { margin-left: 0; justify-content: flex-end; }
  .shortcut { display: none; }
  .metric-chart { height: 320px; }
  .log-content { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 6px 10px; }
  .line-text { grid-column: 1 / -1; }
  .log-content time, .severity { flex: none; }
}
</style>

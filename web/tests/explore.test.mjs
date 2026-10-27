import test from 'node:test'
import assert from 'node:assert/strict'
import { queryRange,queryStep,highlightParts,logLevel,appendLogFilter,metricSeries } from '../src/components/explore/query-utils.mjs'
test('relative time advances on refresh; invalid ranges and oversized steps are rejected',()=>{
 assert.deepEqual(queryRange(300,[],1000000).map(Number),[700000,1000000])
 assert.deepEqual(queryRange(300,[],1010000).map(Number),[710000,1010000])
 assert.throws(()=>queryRange(0,[new Date(2000),new Date(1000)]))
 assert.throws(()=>queryRange(32*86400,[]))
 assert.throws(()=>queryStep(queryRange(86400,[],100000000),1))
 assert.equal(queryStep(queryRange(3600,[],100000000)),5)
})
test('log highlighting treats HTML and regex characters as literal text',()=>{
 const line='<script>alert(1)</script> error.* ERROR.*'
 const parts=highlightParts(line,'error.*')
 assert.equal(parts.map(part=>part.text).join(''),line)
 assert.equal(parts.filter(part=>part.match).length,2)
 assert.equal(highlightParts(line,'')[0].text,line)
})
test('log filters quote values and preserve existing pipeline',()=>{
 const text='error" \\ \n 中文'
 assert.equal(appendLogFilter('{app="api"} | json','loki','',text),'{app="api"} | json |= '+JSON.stringify(text))
 assert.equal(appendLogFilter('{app="api"}','loki','level','error'),'{app="api"} | level="error"')
 assert.throws(()=>appendLogFilter('{}','loki','invalid|key','value'))
 assert.equal(appendLogFilter('*','elasticsearch','service.name','api'), '(*) AND service.name:"api"')
 assert.equal(logLevel({labels:{level:'WARNING'},line:'error'}),'warn')
})
test('metric scalar, instant and range results remain plottable',()=>{
 assert.deepEqual(metricSeries({resultType:'scalar',result:[1,'2']},'A')[0].data,[[1,'2']])
 const result=metricSeries({resultType:'vector',result:[{metric:{job:'api'},value:[1,'3']}]},'B')
 assert.equal(result[0].name,'B · job="api"')
 assert.deepEqual(result[0].data,[[1,'3']])
 assert.equal(metricSeries({resultType:'matrix',result:[]},'A').length,0)
})
import { decodeQueryState } from '../src/components/explore/query-utils.mjs'
test('shared query state validates external URL input',()=>{
 assert.equal(decodeQueryState('bad'),null)
 assert.equal(decodeQueryState(JSON.stringify({queries:[{text:3}],duration:300})),null)
 assert.equal(decodeQueryState(JSON.stringify({queries:[{text:'up'}],duration:-1})),null)
 const state=decodeQueryState(JSON.stringify({queries:[{text:'up',enabled:true}],duration:3600,queryType:'instant'}))
 assert.equal(state.queries[0].text,'up')
 assert.equal(state.queryType,'instant')
})

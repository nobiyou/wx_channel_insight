import test from 'node:test';
import assert from 'node:assert/strict';
import { fetchOperationsAgent, inspectOperationsAgent, decideOperationsAgent } from './api.js';

test('agent API sends only scope and proposal IDs and preserves server failures',async()=>{
 const original=globalThis.fetch;const calls=[];
 try{
  globalThis.fetch=async(path,options)=>{calls.push({path,options});return {ok:true,json:async()=>({runs:[],proposals:[]})}};
  await fetchOperationsAgent();await inspectOperationsAgent({keyword:'秋天',analyze:true});await decideOperationsAgent('apply',['saved-proposal']);
  assert.equal(calls[0].options.cache,'no-store');
  assert.ok(calls[1].path.endsWith('/operations-agent/inspect'));
  assert.deepEqual(JSON.parse(calls[2].options.body),{ids:['saved-proposal']});
  await assert.rejects(decideOperationsAgent('delete',[]));assert.equal(calls.length,3);
  globalThis.fetch=async()=>({ok:false,status:503,json:async()=>({error:'需要 PostgreSQL'})});
  await assert.rejects(fetchOperationsAgent(),/需要 PostgreSQL/);
 }finally{globalThis.fetch=original}
});

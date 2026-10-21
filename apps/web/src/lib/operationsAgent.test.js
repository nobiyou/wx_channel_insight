import test from 'node:test';
import assert from 'node:assert/strict';
import { agentQueue, agentSummary, agentConfirmation, agentAIView, agentEvents } from './operationsAgent.js';
import { parseOperationsRoute, buildOperationsHash } from './operationsNavigation.js';

test('agent queues distinguish missing data from editorial rejection and resolved records',()=>{
 const rows=[{id:'a',action:'selected',status:'pending'},{id:'b',action:'refresh',status:'pending'},{id:'c',action:'excluded',status:'stale'},{id:'d',action:'watching',status:'applied'}];
 assert.deepEqual(agentQueue(rows,'pending').map(v=>v.id),['a']);
 assert.deepEqual(agentQueue(rows,'quality').map(v=>v.id),['b']);
 assert.deepEqual(agentSummary(rows),{selected:1,watching:0,excluded:0,refresh:1,historical:0});
 assert.equal(agentConfirmation(rows,['a','b','c']).count,1);
 assert.equal(agentConfirmation(rows,['a','b','c']).refresh,0);
});

test('historical references cannot enter current opportunities or a bulk curation confirmation',()=>{
 const rows=[{id:'old',action:'historical',status:'pending'},{id:'new',action:'selected',status:'pending'}];
 assert.deepEqual(agentQueue(rows,'pending').map(p=>p.id),['new']);
 assert.deepEqual(agentQueue(rows,'cases').map(p=>p.id),['old']);
 assert.deepEqual(agentConfirmation(rows,['old','new']).ids,['new']);
 assert.equal(agentSummary(rows).historical,1);
});
test('only persisted completed model results are described as AI analysis',()=>{
 assert.equal(agentAIView({aiStatus:'fallback',aiResult:{summary:'invented'}}).result,null);
 assert.equal(agentAIView({aiStatus:'failed'}).label,'AI 未完成 · 规则建议可用');
 assert.equal(agentAIView({aiStatus:'completed',aiResult:{summary:'ok'}}).result.summary,'ok');
 assert.equal(agentAIView(null).label,'等待首次巡检');
});
test('audit events keep chronology and video links return to agent',()=>{
 const events=agentEvents([{id:'a',events:[{at:'2026-10-19T01:00:00Z',action:'apply'},{at:'2026-10-19T02:00:00Z',action:'undo'}]}]);
 assert.equal(events[0].action,'undo');assert.equal(events[0].proposal.id,'a');
 assert.equal(parseOperationsRoute('#agent').view,'agent');
 assert.equal(parseOperationsRoute(buildOperationsHash('video',{id:'a',return:'#agent'})).returnHash,'#agent');
});

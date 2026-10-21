export const agentActionLabels = Object.freeze({selected:'近期精选',watching:'增长观察',excluded:'建议排除',refresh:'重点补采',historical:'历史案例'});
export const agentStatusLabels = Object.freeze({pending:'待确认',applied:'已执行',dismissed:'已忽略',stale:'建议已失效',undone:'已撤销',conflict:'存在后续修改',failed:'未完成',busy:'采集忙碌',cooldown:'等待重试',fresh:'数据已新鲜',updated:'已采集',incomplete:'指标待补齐',needs_refresh:'等待补采'});
export function agentQueue(proposals=[],tab='pending',action='') {
 const allowed=tab==='quality'?['refresh']:tab==='cases'?['historical']:['selected','watching','excluded'];
 return proposals.filter(p=>p.status==='pending'&&allowed.includes(p.action)&&(!action||p.action===action)).sort((a,b)=>new Date(b.createdAt)-new Date(a.createdAt));
}
export function agentSummary(proposals=[]) {
 const result={selected:0,watching:0,excluded:0,refresh:0,historical:0};for(const p of proposals)if(p.status==='pending'&&p.action in result)result[p.action]++;return result;
}
export function agentConfirmation(proposals=[],ids=[]) {
 const rows=agentQueue(proposals).filter(p=>ids.includes(p.id));
 return {...agentSummary(rows),count:rows.length,ids:rows.map(p=>p.id)};
}
export function agentAIView(run) {
 if(!run)return {label:'等待首次巡检',result:null};
 const labels={not_requested:'规则巡检',running:'AI 研判中',completed:'AI 辅助研判',fallback:'规则建议 · AI 未启用',failed:'AI 未完成 · 规则建议可用',disabled:'规则建议 · AI 已停用',insufficient_evidence:'规则建议 · AI 证据不足'};
 return {label:labels[run.aiStatus]||'规则建议',result:run.aiStatus==='completed'?run.aiResult:null};
}
export function agentEvents(proposals=[]) {
 return proposals.flatMap(proposal=>(proposal.events||[]).map((event,index)=>({...event,proposal,key:`${proposal.id}-${index}`}))).sort((a,b)=>new Date(b.at)-new Date(a.at));
}

<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import { NAlert, NModal, NSpin } from 'naive-ui';
import { ArrowUpRight, Bot, CheckCheck, CircleCheck, Clock3, Database, History, RefreshCw, ShieldCheck, Sparkles, Undo2 } from 'lucide-vue-next';
import { fetchOperationsAgent, inspectOperationsAgent, decideOperationsAgent } from '../lib/api.js';
import { agentActionLabels, agentStatusLabels, agentQueue, agentSummary, agentConfirmation, agentAIView, agentEvents } from '../lib/operationsAgent.js';
import { formatMonitoringTime } from '../lib/monitoringTasks.js';
import { formatInteger } from '../lib/numberFormatting.js';

const emit=defineEmits(['navigate','open-video']);
const state=ref({runs:[],proposals:[]});
const loading=ref(false), busy=ref(''), error=ref(''), notice=ref('');
const keyword=ref(''),account=ref(''),analyze=ref(true);
const tab=ref('pending'),actionFilter=ref(''),selected=ref([]),page=ref(1),showConfirm=ref(false);
const lastRun=computed(()=>state.value.runs?.[0]);
const quality=computed(()=>lastRun.value?.quality);
const summary=computed(()=>agentSummary(state.value.proposals));
const queue=computed(()=>agentQueue(state.value.proposals,tab.value,actionFilter.value));
const records=computed(()=>agentEvents(state.value.proposals));
const pages=computed(()=>Math.max(1,Math.ceil((tab.value==='history'?records.value.length:queue.value.length)/20)));
const visible=computed(()=>queue.value.slice((page.value-1)*20,page.value*20));
const visibleRecords=computed(()=>records.value.slice((page.value-1)*20,page.value*20));
const confirmation=computed(()=>agentConfirmation(state.value.proposals,selected.value));
const currentRules=computed(()=>!state.value.ruleVersion||lastRun.value?.ruleVersion===state.value.ruleVersion);
const ai=computed(()=>currentRules.value?agentAIView(lastRun.value):{label:'运营规则已升级',result:null});
const queueDescriptions={pending:'近期精选与排除仅针对近 30 天发布的内容。老视频只有近期可比增量达到门槛才进入增长观察；确认前重新核对证据。',quality:'重点补采近 30 天内容及人工精选、观察的视频。普通历史库存不因陈旧或缺失自动进入补采队列；每次补采保留结果。',cases:'发布超过 30 天的累计高表现案例，仅供选题与表达方式复盘，不代表当前热门，不参与批量精选确认。'};
const allSelected=computed(()=>visible.value.length>0&&visible.value.every(p=>selected.value.includes(p.id)));
const stats=computed(()=>[
 {key:'selected',label:'近期精选',description:'近 30 天发布，累计表现突出',icon:Sparkles},
 {key:'watching',label:'待观察',description:'互动增长，继续跟踪变化',icon:Clock3},
 {key:'excluded',label:'建议排除',description:'证据充分，移出有效内容池',icon:ShieldCheck},
 {key:'refresh',label:'重点补采',description:'近期内容与已关注视频优先',icon:Database},
]);
const number=v=>v===null||v===undefined?'—':formatInteger(v);
const time=v=>!v||String(v).startsWith('0001-')?'时间未知':formatMonitoringTime(v,'时间未知');
const label=status=>agentStatusLabels[status]||status;
const undoable=p=>p.status==='dismissed'||p.status==='applied'&&p.action!=='refresh'&&p.after;
watch([tab,actionFilter],()=>{page.value=1;selected.value=[]});
watch(pages,n=>{if(page.value>n)page.value=n});
function changeTab(next){tab.value=next;actionFilter.value=''}
function filterAction(key){tab.value=key==='refresh'?'quality':'pending';actionFilter.value=key;selected.value=[];page.value=1}
function selectAll(){selected.value=allSelected.value?selected.value.filter(id=>!visible.value.some(p=>p.id===id)):[...new Set([...selected.value,...visible.value.map(p=>p.id)])].slice(0,50)}
function toggle(id){selected.value=selected.value.includes(id)?selected.value.filter(v=>v!==id):[...selected.value,id].slice(0,50)}
function openVideo(p){emit('open-video',{video:{objectId:p.objectId},returnHash:'#agent'})}
async function load(options={}){
 if(loading.value||options.background&&(busy.value||showConfirm.value))return;
 loading.value=true;
 try{state.value=await fetchOperationsAgent();error.value='';selected.value=selected.value.filter(id=>state.value.proposals.some(p=>p.id===id&&p.status==='pending'))}
 catch(err){error.value=err.message||'智能运营暂不可用'}finally{loading.value=false}
}
async function inspect(){
 if(busy.value)return;
 busy.value='inspect';error.value='';notice.value='';
 try{
  const result=await inspectOperationsAgent({keyword:keyword.value.trim(),account:account.value.trim(),analyze:analyze.value});
  state.value=result.state;selected.value=[];
  const run=lastRun.value;
  notice.value=result.reused?'数据未变化，已复用本轮巡检和研判。':`已巡检 ${number(run.quality.total)} 条视频，新增 ${run.proposed} 条建议。每轮最多优先处理 20 条。`;
  if(!summary.value.selected&&!summary.value.watching&&!summary.value.excluded&&summary.value.refresh)changeTab('quality');
 }catch(err){error.value=err.message||'巡检未完成，已保存的数据可稍后读取'}finally{busy.value=''}
}
async function decide(action,ids){
 if(busy.value||!ids.length)return;
 busy.value=action;error.value='';notice.value='';
 try{
  const result=await decideOperationsAgent(action,ids);
  if(action==='refresh')notice.value=result.message||label(result.status);
  else{
   const failures=(result.items||[]).filter(p=>['stale','conflict','needs_refresh','reference_only'].includes(p.status));
   notice.value=failures.length?`${failures.length} 条因证据变化或后续修改未执行，请查看执行记录。`:`已处理 ${result.items?.length||0} 条建议。`;
  }
  selected.value=[];await load();
 }catch(err){error.value=err.message||'操作未完成，请更新数据核对记录'}finally{busy.value='';showConfirm.value=false}
}
function applyConfirmed(){decide('apply',confirmation.value.ids)}
onMounted(load);defineExpose({refresh:load});
</script>

<template>
 <section class="analysis-page agent-page" :aria-busy="loading||!!busy">
  <div class="analysis-page-head"><div><h2>让采集数据变成运营行动</h2><p>采集成功后自动巡检，新证据触发 AI 研判；精选和排除由你确认。</p></div><span class="operations-updated"><n-spin v-if="loading" size="small"/><Clock3 v-else :size="14"/>{{lastRun?`最近${lastRun.trigger==='collection'?'自动':'手动'}巡检 ${time(lastRun.createdAt)}`:'等待采集后首次自动巡检'}}</span></div>
  <n-alert v-if="error" type="error" show-icon>{{error}}<button class="operations-text-button" type="button" :disabled="loading" @click="load()">重新读取</button></n-alert>
  <n-alert v-if="notice" type="info" show-icon aria-live="polite">{{notice}}</n-alert>
  <section class="analysis-surface agent-control">
   <div class="agent-control-head"><div class="agent-heading"><span class="agent-bot"><Bot :size="22"/></span><div><h3>运营巡检助手</h3><p>区分近期机会、老视频增长与历史案例；最近采集不等于最近发布。</p></div></div><span class="operations-badge" data-tone="good"><ShieldCheck :size="13"/>人工确认后执行</span></div>
   <form class="operations-filter-form" @submit.prevent="inspect">
    <label><span>关键词范围</span><input v-model="keyword" maxlength="128" placeholder="留空检查全部；按采集关键词精确匹配" :disabled="!!busy"/></label>
    <label><span>账号范围</span><input v-model="account" maxlength="128" placeholder="账号名称或 ID，可留空" :disabled="!!busy"/></label>
    <button class="tool-button primary agent-inspect" type="submit" :disabled="!!busy||loading"><n-spin v-if="busy==='inspect'" :size="15"/><Sparkles v-else :size="16"/>{{busy==='inspect'?'巡检与研判中…':'开始巡检'}}</button>
   </form>
   <div class="agent-control-foot"><label class="agent-check"><input v-model="analyze" type="checkbox" :disabled="!!busy"/><span>本次手动巡检使用 AI 补充研判</span></label><span>自动巡检使用已配置 AI · 无待研判建议时跳过 · 同份证据复用结果</span></div>
   <details class="agent-rule"><summary>查看判断依据与时间范围</summary><ul><li>近期精选：近 30 天发布，24 小时内采集且四项互动完整；同账号同发布龄至少 5 条样本，累计互动达到中位数 2 倍且至少 100 次，或在至少 10 条有效关键词样本中位于前 10%。</li><li>历史案例：超过 30 天的累计高表现内容，仅供复盘。比较组未校正事件背景和账号规模，不代表内容质量或当前热度。</li><li>老视频增长观察：近 72 小时内两次可比采集，间隔至少 10 分钟，最新指标在 24 小时内更新；新增至少 100 次且达到基线 1%。这是一条观察线索，不证明持续走热。</li><li>排除建议：限近 30 天发布且已满 3 天、指标完整新鲜、有可比历史、累计互动不超过同组中位数 30% 的内容；始终需要人工确认。</li></ul></details>
   <div v-if="lastRun" class="agent-quality-line"><strong>{{lastRun.scope}}</strong><span>检查 {{number(quality.total)}} 条</span><span>新鲜 {{number(quality.fresh)}}</span><span :class="{'agent-warning':quality.stale}">待更新 {{number(quality.stale)}}</span><span :class="{'agent-warning':quality.missing}">互动缺失 {{number(quality.missing)}}</span><span v-if="quality.anomalous">计数回调 {{number(quality.anomalous)}}</span><span>已保护人工标记 {{number(quality.protected)}}</span><small>质量项可重叠</small></div>
   <n-alert v-if="lastRun?.capacityReached" type="warning">建议记录已达容量上限。历史记录完整保留，本轮未继续添加新建议。</n-alert>
  </section>
  <div class="agent-stat-grid"><button v-for="stat in stats" :key="stat.key" class="analysis-surface agent-stat" type="button" :data-action="stat.key" @click="filterAction(stat.key)"><span><component :is="stat.icon" :size="17"/>{{stat.label}}<ArrowUpRight :size="14"/></span><strong>{{lastRun?number(summary[stat.key]):'—'}}</strong><small>{{stat.description}}</small></button></div>
  <section v-if="lastRun" class="analysis-surface agent-ai">
   <div class="agent-ai-head"><span><Sparkles :size="17"/><strong>{{ai.label}}</strong><small v-if="lastRun.aiModel">{{lastRun.aiModel}}</small></span><button type="button" class="operations-text-button" @click="emit('navigate','settings',{tab:'ai'})">AI 设置<ArrowUpRight :size="13"/></button></div>
   <p>{{currentRules?(ai.result?.summary||lastRun.aiReason):'旧版未确认建议已失效并保留记录。请重新巡检，按发布时间与近期增长区分运营机会和历史参考。'}}</p>
   <details v-if="ai.result" class="agent-ai-expand"><summary>查看研判建议与分析限制</summary><div class="agent-ai-details"><div v-if="ai.result.recommendations?.length"><h4>建议关注</h4><ul><li v-for="(item,i) in ai.result.recommendations" :key="i">{{item}}</li></ul></div><div v-if="ai.result.risks?.length"><h4>分析限制</h4><ul><li v-for="(item,i) in ai.result.risks" :key="i">{{item}}</li></ul></div></div></details>
   <small>AI 文字仅作辅助解释，实际建议依据规则生成；模型不会直接改变精选或排除状态。</small>
  </section>
  <section class="analysis-surface agent-workbench">
   <div class="agent-tabs" role="tablist" aria-label="运营事项"><button v-for="item in [{key:'pending',label:'近期机会',icon:CheckCheck},{key:'quality',label:'数据问题',icon:Database},{key:'cases',label:'历史案例',icon:History},{key:'history',label:'执行记录',icon:History}]" :key="item.key" type="button" role="tab" :aria-selected="tab===item.key" :class="{active:tab===item.key}" @click="changeTab(item.key)"><component :is="item.icon" :size="16"/>{{item.label}}<span>{{item.key==='history'?records.length:agentQueue(state.proposals,item.key).length}}</span></button></div>
   <template v-if="tab!=='history'">
    <div class="agent-queue-head"><p>{{queueDescriptions[tab]}}</p><select v-if="tab==='pending'" v-model="actionFilter" aria-label="建议类型"><option value="">全部建议</option><option value="selected">近期精选</option><option value="watching">增长观察</option><option value="excluded">建议排除</option></select></div>
    <div v-if="queue.length" class="agent-batch"><label v-if="tab==='pending'" class="agent-check"><input type="checkbox" :checked="allSelected" :disabled="!!busy" @change="selectAll"/>本页全选</label><span v-if="tab==='pending'">已选 {{selected.length}} / 50</span><span v-else>{{tab==='cases'?'历史参考':'重点补采'}} · {{queue.length}} 条</span><button v-if="tab==='pending'" class="tool-button primary" type="button" :disabled="!confirmation.count||!!busy" @click="showConfirm=true">确认所选建议</button><button v-if="tab==='pending'" class="tool-button secondary" type="button" :disabled="!selected.length||!!busy" @click="decide('dismiss',selected)">忽略所选</button></div>
    <div v-if="queue.length" class="agent-candidates"><article v-for="p in visible" :key="p.id" class="agent-candidate" :data-action="p.action">
     <div class="agent-candidate-top"><input v-if="tab==='pending'" type="checkbox" :checked="selected.includes(p.id)" :disabled="!!busy||selected.length>=50&&!selected.includes(p.id)" :aria-label="`选择建议：${p.title||p.videoId}`" @change="toggle(p.id)"/><span class="agent-action" :data-action="p.action">{{agentActionLabels[p.action]}}</span><button class="agent-video-title" type="button" @click="openVideo(p)">{{p.title||'未命名视频'}}</button><span class="agent-candidate-account">{{p.account||'账号待确认'}}</span></div>
     <div class="agent-candidate-body"><div class="agent-evidence"><p>{{p.reason}}</p><div class="agent-evidence-meta"><span>累计互动 <strong>{{number(p.performance?.interactions)}}</strong></span><span v-if="p.performance?.relativeToMedian!=null">同账号中位数 <strong>{{Number(p.performance.relativeToMedian).toFixed(2)}} 倍</strong></span><span v-if="p.performance?.growthPerHour!=null">新增 <strong>{{Number(p.performance.growthPerHour).toFixed(1)}} / 小时</strong></span><span v-for="kw in p.keywords||[]" :key="kw" class="agent-keyword">{{kw}}</span></div><div class="agent-evidence-time"><span>发布时间：{{time(p.publishedAt)}}</span><span>指标采集：{{time(p.observedAt)}}{{p.performance?.freshness==='stale'?'（历史快照）':''}}</span><span v-if="p.performance?.growthPerHour!=null&&p.growthWindow?.baselineObservedAt">增长窗口：{{time(p.growthWindow.baselineObservedAt)}} 至 {{time(p.growthWindow.latestObservedAt)}}</span></div><p v-if="p.events?.length" class="agent-last-attempt">{{label(p.events.at(-1).status)}} · {{p.events.at(-1).message}}</p></div><div class="agent-candidate-actions"><button v-if="p.action==='refresh'" class="tool-button primary" type="button" :disabled="!!busy" @click="decide('refresh',[p.id])"><RefreshCw :size="14"/>{{busy==='refresh'?'采集中…':'补采指标'}}</button><button class="tool-button secondary" type="button" :disabled="!!busy" @click="openVideo(p)">查看证据<ArrowUpRight :size="14"/></button><button class="operations-text-button" type="button" :disabled="!!busy" @click="decide('dismiss',[p.id])">忽略</button></div></div>
    </article></div>
    <div v-else class="agent-empty"><component :is="tab==='quality'?Database:CircleCheck" :size="34"/><h3>{{!lastRun?'从一次运营巡检开始':tab==='quality'?'暂无重点补采建议':tab==='cases'?'暂无历史案例':'当前没有近期机会建议'}}</h3><p>{{!lastRun?'选择监控范围，检查数据质量并生成可解释的运营建议。':tab==='quality'?'普通历史库存不自动补采；每轮最多生成 20 条事项。':tab==='cases'?'历史案例需要完整的累计互动和足够的对比样本。':'没有近期证据时不使用历史高互动视频凑数。可采集近期内容后重新巡检，或到历史案例中复盘。'}}</p><button v-if="lastRun&&tab==='pending'&&summary.refresh" class="tool-button secondary" type="button" @click="changeTab('quality')">处理数据问题</button></div>
   </template>
   <template v-else>
    <p class="agent-record-note">确认、忽略、失效、补采及撤销均保留操作人和时间。后续人工修改会阻止撤销覆盖。</p>
    <div v-if="records.length" class="agent-records"><article v-for="event in visibleRecords" :key="event.key" class="agent-record"><div class="agent-record-head"><span class="operations-badge" :data-tone="['failed','conflict','stale','incomplete'].includes(event.status)?'warn':'good'">{{label(event.status)}}</span><button type="button" @click="openVideo(event.proposal)">{{event.proposal.title||event.proposal.videoId}}</button><time>{{time(event.at)}}</time></div><p>{{event.message}}</p><div class="agent-record-foot"><span>{{event.action==='refresh'?'补采指标':event.action==='undo'?'撤销':event.action==='dismiss'?'忽略':event.action==='recheck'?'重新核验':agentActionLabels[event.proposal.action]}} · {{event.actor}}</span><button v-if="['apply','dismiss'].includes(event.action)&&undoable(event.proposal)" type="button" class="operations-text-button" :disabled="!!busy" @click="decide('undo',[event.proposal.id])"><Undo2 :size="14"/>{{event.action==='dismiss'?'恢复建议':'撤销这次调整'}}</button></div></article></div>
    <div v-else class="agent-empty"><History :size="34"/><h3>尚无执行记录</h3><p>处理建议后，这里会显示执行结果与撤销入口。</p></div>
   </template>
   <div v-if="pages>1" class="agent-pagination"><button class="tool-button secondary" type="button" :disabled="page<=1" @click="page--">上一页</button><span>{{page}} / {{pages}}</span><button class="tool-button secondary" type="button" :disabled="page>=pages" @click="page++">下一页</button></div>
  </section>
  <details v-if="state.runs.length" class="analysis-surface agent-runs"><summary>巡检历史 · 最近 {{state.runs.length}} 轮</summary><div v-for="run in state.runs" :key="run.id"><time>{{time(run.createdAt)}}</time><span>{{run.trigger==='collection'?'采集后自动巡检':'手动巡检'}} · {{run.scope}}</span><span>检查 {{number(run.quality.total)}} 条 · 新增 {{run.proposed}} 条</span><small>{{agentAIView(run).label}}</small></div></details>
  <n-modal v-model:show="showConfirm" preset="card" title="确认调整内容池" class="agent-confirm" :mask-closable="!busy" :closable="!busy" :close-on-esc="!busy" style="width:min(520px,calc(100vw - 32px))"><p>本次确认 {{confirmation.count}} 条建议：精选 {{confirmation.selected}} 条、观察 {{confirmation.watching}} 条、排除 {{confirmation.excluded}} 条。</p><p>执行前会重新核验最新证据和人工标记。排除仅移出有效内容池，原始数据会保留；成功调整可在执行记录中撤销。</p><div class="agent-confirm-actions"><button class="tool-button secondary" type="button" :disabled="!!busy" @click="showConfirm=false">返回检查</button><button class="tool-button primary" type="button" :disabled="!!busy||!confirmation.count" @click="applyConfirmed"><n-spin v-if="busy==='apply'" :size="14"/>{{busy==='apply'?'核验并执行中…':'确认执行'}}</button></div></n-modal>
 </section>
</template>

<style scoped>
.agent-rule{font-size:12px;line-height:1.9;color:var(--ops-muted);margin-top:14px}.agent-rule summary{cursor:pointer;color:#4b6382}.agent-rule ul{padding-left:20px;margin-bottom:0}.agent-rule li+li{margin-top:6px}
.agent-ai-expand{font-size:12px;color:#5c7392;margin:12px 0}.agent-ai-expand summary{cursor:pointer;width:fit-content;color:#3567b7;padding:4px 0}
.agent-page{display:grid;gap:18px}.agent-page .analysis-page-head{margin-bottom:0}.agent-control{padding:22px}.agent-control-head,.agent-heading,.agent-control-foot,.agent-quality-line,.agent-ai-head,.agent-ai-head>span,.agent-batch,.agent-candidate-top,.agent-evidence-meta,.agent-evidence-time,.agent-record-head,.agent-record-foot{display:flex;align-items:center;gap:10px;flex-wrap:wrap}.agent-control-head,.agent-control-foot,.agent-ai-head{justify-content:space-between}.agent-heading{flex-wrap:nowrap;gap:12px;min-width:0}.agent-heading h3{margin:0;font-size:15px}.agent-heading p{margin:5px 0 0;font-size:12px;color:var(--ops-muted);line-height:1.7}.agent-bot{display:grid;place-items:center;flex-shrink:0;width:44px;height:44px;border-radius:12px;background:#eef3ff;color:#2f6fec}.agent-control-head .operations-badge{display:flex;align-items:center;gap:4px}.agent-control-foot{margin-top:15px;font-size:11px;color:var(--ops-muted);line-height:1.7}.agent-check{display:inline-flex;align-items:center;gap:7px;font-size:12px;cursor:pointer}.agent-check input,.agent-candidate-top>input{accent-color:#2f6fec;width:16px;height:16px;flex-shrink:0}.agent-inspect{display:flex;align-items:center;justify-content:center;gap:7px}.agent-quality-line{border-top:1px solid #e9eef5;padding-top:15px;margin-top:15px;color:var(--ops-muted);font-size:12px;gap:9px 18px}.agent-quality-line strong{color:var(--ops-ink)}.agent-quality-line small{font-size:10px}.agent-warning{color:#a45a10}.agent-stat-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:14px}.agent-stat{padding:18px;text-align:left;cursor:pointer;transition:border-color .15s}.agent-stat:hover{border-color:#9ab6ef}.agent-stat>span{display:flex;align-items:center;gap:7px;font-size:12px;color:#52677f}.agent-stat>span svg:last-child{margin-left:auto}.agent-stat strong{display:block;font-size:29px;color:#253854;margin:12px 0 5px;font-variant-numeric:tabular-nums}.agent-stat small{color:var(--ops-muted);font-size:11px;line-height:1.6}.agent-stat[data-action=selected] svg:first-child{color:#976411}.agent-stat[data-action=refresh] svg:first-child{color:#b06b20}.agent-ai{padding:19px 22px;border-left:3px solid #8eaef1}.agent-ai-head>span{font-size:13px;color:#2f5392}.agent-ai-head small{color:var(--ops-muted);font-size:11px}.agent-ai p{font-size:13px;line-height:1.85;margin:12px 0 8px;color:#3e526d}.agent-ai>small{font-size:11px;color:var(--ops-muted);line-height:1.7}.agent-ai-details{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:20px}.agent-ai h4{font-size:12px;margin:8px 0}.agent-ai ul{padding-left:18px;font-size:12px;line-height:1.8;color:#53677f}.agent-workbench{overflow:hidden}.agent-tabs{display:flex;flex-wrap:wrap;border-bottom:1px solid #e5ebf3;padding:0 20px;gap:24px}.agent-tabs button{display:flex;align-items:center;gap:7px;min-height:56px;background:none;border:0;border-bottom:2px solid transparent;color:#63738b;font-size:13px;cursor:pointer;padding:0 2px;white-space:nowrap}.agent-tabs button.active{color:#2f6fec;border-color:#2f6fec;font-weight:600}.agent-tabs button>span{border-radius:12px;padding:1px 7px;background:#f1f4f8;font-size:11px}.agent-queue-head{padding:16px 20px;display:flex;align-items:center;gap:16px;justify-content:space-between}.agent-queue-head p,.agent-record-note{font-size:12px;line-height:1.8;color:var(--ops-muted);margin:0}.agent-queue-head select{min-height:34px;border:1px solid #dce4ef;border-radius:6px;padding:4px 8px;color:#415773;background:#fff}.agent-batch{padding:10px 20px;background:#f6f8fc;border-top:1px solid #edf1f6;border-bottom:1px solid #e4ebf4;gap:12px;font-size:12px;color:#61758e}.agent-batch .tool-button{min-height:32px;padding:6px 12px;font-size:12px}.agent-batch .primary{margin-left:auto}.agent-candidate{padding:20px;border-bottom:1px solid #e9eef5}.agent-candidate:last-child{border:0}.agent-candidate-top{gap:8px 10px;align-items:start}.agent-action{font-size:11px;border-radius:5px;padding:3px 8px;background:#edf3ff;color:#3560ab;white-space:nowrap}.agent-action[data-action=selected]{background:#f9f2df;color:#90651a}.agent-action[data-action=historical]{background:#f0f2f5;color:#66758b}.agent-action[data-action=excluded]{background:#f4edf0;color:#925b6f}.agent-action[data-action=refresh]{background:#fff3e7;color:#9b611d}.agent-video-title{flex:1 1 250px;text-align:left;background:none;border:0;padding:0;color:#233b5b;font-size:14px;font-weight:600;line-height:1.65;overflow-wrap:anywhere;cursor:pointer}.agent-video-title:hover{text-decoration:underline}.agent-candidate-account{color:var(--ops-muted);font-size:12px;line-height:1.9;max-width:180px;overflow-wrap:anywhere}.agent-candidate-body{display:flex;gap:20px;align-items:center;margin:10px 0 0 26px}.agent-evidence{flex:1;min-width:0}.agent-evidence>p{font-size:12px;line-height:1.85;margin:0 0 8px;color:#53677f;overflow-wrap:anywhere}.agent-evidence-meta{font-size:11px;color:#718198;gap:7px 18px}.agent-evidence-meta strong{color:#445c7b;font-weight:600}.agent-keyword{padding:2px 7px;border-radius:4px;background:#f0f4fa;color:#57749a;overflow-wrap:anywhere}.agent-evidence-time{color:#7c8ba0;font-size:11px;margin-top:9px;gap:6px 18px}.agent-candidate-actions{display:flex;align-items:center;gap:8px;flex-wrap:wrap;justify-content:flex-end;max-width:210px}.agent-candidate-actions .tool-button{display:flex;align-items:center;gap:5px;font-size:12px;min-height:32px;padding:6px 10px}.agent-last-attempt{margin-top:10px!important;color:#a16323!important}.agent-empty{display:flex;flex-direction:column;align-items:center;padding:45px 22px;text-align:center;color:#7f95af;gap:12px}.agent-empty h3{color:#3d5472;font-size:15px;margin:0}.agent-empty p{font-size:12px;color:var(--ops-muted);line-height:1.9;max-width:500px;margin:0}.agent-record-note{padding:17px 20px}.agent-record{padding:17px 20px;border-top:1px solid #edf1f6}.agent-record-head button{flex:1 1 220px;text-align:left;background:none;border:0;padding:0;line-height:1.7;font-size:13px;color:#365579;cursor:pointer;overflow-wrap:anywhere}.agent-record-head time{font-size:11px;color:var(--ops-muted)}.agent-record>p{font-size:12px;color:#657993;line-height:1.8;margin:9px 0;overflow-wrap:anywhere}.agent-record-foot{justify-content:space-between;color:#8593a6;font-size:11px;overflow-wrap:anywhere}.agent-record-foot .operations-text-button{display:flex;gap:5px;align-items:center;font-size:12px}.agent-pagination{display:flex;align-items:center;justify-content:flex-end;gap:13px;padding:15px 20px;border-top:1px solid #edf1f6;color:#73849a;font-size:12px}.agent-runs{padding:17px 20px;color:#62768f;font-size:12px}.agent-runs summary{cursor:pointer;color:#4b6382;font-weight:600}.agent-runs>div{display:flex;gap:12px 20px;flex-wrap:wrap;padding-top:13px;font-size:11px}.agent-runs small{color:#8d9aae}.agent-confirm p{font-size:13px;line-height:1.9;color:#52677f}.agent-confirm-actions{display:flex;justify-content:flex-end;gap:10px;margin-top:20px}.agent-page button:focus-visible,.agent-page input:focus-visible,.agent-page select:focus-visible{outline:2px solid #2f6fec;outline-offset:3px}
@media(max-width:1000px){.agent-stat-grid{grid-template-columns:repeat(2,minmax(0,1fr))}.agent-candidate-body{align-items:start;flex-direction:column;gap:12px}.agent-candidate-actions{max-width:none}.agent-candidate-account{max-width:none}.agent-ai-details{grid-template-columns:1fr}}
@media(max-width:640px){.agent-control{padding:16px}.agent-control-head{align-items:start;gap:12px}.agent-heading{align-items:start}.agent-bot{width:36px;height:36px}.agent-control-head .operations-badge{margin-left:48px}.agent-page{gap:14px}.agent-stat-grid{gap:10px}.agent-stat{padding:14px}.agent-stat strong{font-size:25px;margin-top:10px}.agent-stat small{font-size:10px}.agent-tabs{padding:0 12px;gap:16px}.agent-tabs button{font-size:12px;gap:4px;min-height:50px}.agent-tabs button svg{width:14px}.agent-tabs button>span{padding:1px 5px}.agent-ai{padding:16px}.agent-queue-head{padding:14px;align-items:start;flex-direction:column;gap:10px}.agent-batch{padding:10px 14px;gap:10px}.agent-batch .primary{margin-left:0}.agent-candidate{padding:16px 14px}.agent-video-title{flex-basis:180px;font-size:13px}.agent-candidate-account{margin-left:0;width:100%;font-size:11px}.agent-candidate-body{margin-left:0}.agent-record,.agent-record-note{padding:14px}.agent-record-head time{width:100%}.agent-evidence-time{display:grid}.agent-inspect{width:100%}.agent-control .operations-filter-form label{flex-basis:100%}.agent-quality-line{gap:8px 14px}.agent-confirm-actions .tool-button{flex:1}.agent-tabs{gap:12px}}
</style>

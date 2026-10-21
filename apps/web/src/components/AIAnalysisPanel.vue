<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { AlertTriangle, CheckCircle2, Clock3, History, Play, Sparkles } from "lucide-vue-next";
import { fetchAIAnalysisRuns, runAIAnalysis } from "../lib/api.js";
import {
  buildAIAnalysisPayload,
  formatAIAnalysisTime,
  getAIAnalysisModuleLabel,
  normalizeAIAnalysisRun,
} from "../lib/aiAnalysis.js";

const props = defineProps({
  module: { type: String, required: true },
  scopeKey: { type: String, default: "dashboard" },
  window: { type: String, default: "all" },
  variables: { type: Object, default: () => ({}) },
  title: { type: String, default: "AI 分析建议" },
  runLabel: { type: String, default: "运行 AI 分析" },
  historyLimit: { type: Number, default: 5 },
  autoRun: { type: Boolean, default: false },
  readOnly: { type: Boolean, default: false },
});

const latestRun = ref(null);
const history = ref([]);
const busy = ref(false);
const loadingHistory = ref(false);
const error = ref("");

const moduleLabel = computed(() => getAIAnalysisModuleLabel(props.module));
const panelTitle = computed(() => props.title || `${moduleLabel.value}建议`);
const currentStatus = computed(() => latestRun.value?.statusMeta || null);
const canShowResult = computed(() => Boolean(
  latestRun.value?.status === "completed" && latestRun.value?.resultSections?.length,
));

function normalizedRun(run) {
  return normalizeAIAnalysisRun(props.module, run);
}

function setHistory(payload) {
  const items = Array.isArray(payload?.items) ? payload.items : [];
  history.value = items.map(normalizedRun).filter(Boolean);
  if (!latestRun.value && history.value.length) latestRun.value = history.value[0];
}

async function loadHistory() {
  loadingHistory.value = true;
  try {
    const payload = await fetchAIAnalysisRuns(props.module, {
      scopeKey: props.scopeKey,
      window: props.window,
      limit: props.historyLimit,
    });
    setHistory(payload);
    if (props.autoRun && history.value.length === 0) {
      await executeAnalysis();
    }
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "AI 分析历史暂时无法读取";
  } finally {
    loadingHistory.value = false;
  }
}

async function executeAnalysis() {
  busy.value = true;
  error.value = "";
  try {
    const payload = await runAIAnalysis(
      props.module,
      buildAIAnalysisPayload(props.module, {
        scopeKey: props.scopeKey,
        window: props.window,
        variables: props.variables,
      }),
    );
    const nextRun = normalizedRun(payload);
    if (!nextRun) throw new Error("AI 分析返回结果无法识别");
    latestRun.value = nextRun;
    history.value = [nextRun, ...history.value.filter((item) => item?.id !== nextRun.id)];
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : "AI 分析暂时无法运行";
  } finally {
    busy.value = false;
  }
}

function selectHistory(run) {
  latestRun.value = run;
  error.value = "";
}

function formatStatusIcon(tone) {
  return tone === "good" ? CheckCircle2 : AlertTriangle;
}

watch(
  () => [props.module, props.scopeKey, props.window],
  () => {
    latestRun.value = null;
    history.value = [];
    error.value = "";
    void loadHistory();
  },
);

onMounted(() => {
  void loadHistory();
});
</script>

<template>
  <section class="ai-analysis-panel" :aria-labelledby="`ai-analysis-${module}-title`">
    <div class="ai-analysis-panel-head">
      <div class="ai-analysis-panel-heading">
        <span class="ai-analysis-kicker"><Sparkles :size="14" aria-hidden="true" /> AI 建议层</span>
        <h3 :id="`ai-analysis-${module}-title`">{{ panelTitle }}</h3>
        <p>{{ moduleLabel }} · {{ scopeKey }} · {{ window || "all" }}</p>
      </div>
      <button
        v-if="!readOnly"
        class="ai-analysis-run-button"
        type="button"
        :disabled="busy"
        @click="executeAnalysis"
      >
        <Play v-if="!busy" :size="15" aria-hidden="true" />
        <Clock3 v-else :size="15" class="is-spinning" aria-hidden="true" />
        <span>{{ busy ? "分析中…" : runLabel }}</span>
      </button>
    </div>

    <div v-if="error" class="ai-analysis-inline-state is-error" role="alert">
      <AlertTriangle :size="16" aria-hidden="true" />
      <span>{{ error }}</span>
      <button type="button" class="ai-analysis-retry" @click="loadHistory">重试</button>
    </div>

    <div v-if="latestRun" class="ai-analysis-run">
      <div class="ai-analysis-status-row">
        <div class="ai-analysis-status-copy">
          <component :is="formatStatusIcon(currentStatus?.tone)" :size="17" aria-hidden="true" />
          <strong>{{ currentStatus?.label || "状态未知" }}</strong>
          <span>{{ currentStatus?.detail }}</span>
        </div>
        <span class="status-badge" :data-tone="currentStatus?.tone || 'risk'">
          {{ latestRun.status === "completed" ? "建议结果" : "无结果" }}
        </span>
      </div>

      <div v-if="canShowResult" class="ai-analysis-result-grid">
        <article v-for="section in latestRun.resultSections" :key="section.key" class="ai-analysis-result-section">
          <div class="ai-analysis-result-label">{{ section.label }}</div>
          <p v-if="section.type === 'text'">{{ section.value }}</p>
          <ul v-else>
            <li v-for="item in section.value" :key="item">{{ item }}</li>
          </ul>
        </article>
      </div>
      <div v-else class="ai-analysis-no-result">
        <AlertTriangle :size="17" aria-hidden="true" />
        <span>{{ latestRun.fallbackReason || currentStatus?.detail || "本次没有可展示的模型建议。" }}</span>
      </div>

      <div v-if="latestRun.evidenceRefs?.length" class="ai-analysis-evidence">
        <div class="ai-analysis-subhead">证据引用</div>
        <div class="ai-analysis-evidence-list">
          <span v-for="reference in latestRun.evidenceRefs" :key="reference">{{ reference }}</span>
        </div>
      </div>

      <div v-if="latestRun.trace?.length" class="ai-analysis-trace">
        <div class="ai-analysis-subhead">运行记录</div>
        <dl class="ai-analysis-trace-grid">
          <div v-for="item in latestRun.trace" :key="item.key">
            <dt>{{ item.label }}</dt>
            <dd :title="item.value">{{ item.value }}</dd>
          </div>
        </dl>
      </div>
    </div>
    <div v-else class="ai-analysis-empty">
      <Sparkles :size="18" aria-hidden="true" />
      <span>{{ loadingHistory ? "正在读取历史运行…" : "暂无 AI 运行记录" }}</span>
    </div>

    <details v-if="history.length" class="ai-analysis-history">
      <summary><History :size="15" aria-hidden="true" /> 历史运行（{{ history.length }}）</summary>
      <div class="ai-analysis-history-list">
        <button
          v-for="run in history"
          :key="run.id"
          type="button"
          class="ai-analysis-history-item"
          :class="{ active: latestRun?.id === run.id }"
          @click="selectHistory(run)"
        >
          <span class="status-badge" :data-tone="run.statusMeta.tone">{{ run.statusMeta.label }}</span>
          <span>{{ formatAIAnalysisTime(run.runAt) }}</span>
          <span>{{ run.window || "all" }}</span>
        </button>
      </div>
    </details>
  </section>
</template>

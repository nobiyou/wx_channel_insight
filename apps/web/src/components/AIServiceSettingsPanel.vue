<script setup>
import { computed, onMounted, reactive, ref, watch } from "vue";
import {
  Bot,
  CheckCircle2,
  CircleAlert,
  Eye,
  EyeOff,
  KeyRound,
  LoaderCircle,
  Save,
  ShieldCheck,
  TestTube2,
  Trash2,
} from "lucide-vue-next";
import {
  clearAIServiceSettingsKey,
  fetchAIServiceSettings,
  updateAIServiceSettings,
  verifyAIServiceSettings,
} from "../lib/api.js";
import {
  AI_SERVICE_DEFAULTS,
  aiServiceSourceLabel,
  buildAIServiceDraftPayload,
  normalizeAIServiceSettings,
  validateAIServiceDraft,
  verificationResultMeta,
  verificationStatusMeta,
} from "../lib/aiServiceSettings.js";

const metadata = ref(normalizeAIServiceSettings());
const draft = reactive({
  provider: AI_SERVICE_DEFAULTS.provider,
  baseUrl: AI_SERVICE_DEFAULTS.baseUrl,
  model: AI_SERVICE_DEFAULTS.model,
  timeoutSeconds: AI_SERVICE_DEFAULTS.timeoutSeconds,
  analysisEngine: AI_SERVICE_DEFAULTS.analysisEngine,
  fallbackPolicy: AI_SERVICE_DEFAULTS.fallbackPolicy,
});
const apiKeyDraft = ref("");
const showAPIKey = ref(false);
const loading = ref(true);
const source = ref("loading");
const busyAction = ref("");
const errorMessage = ref("");
const feedback = ref(null);
const verificationResult = ref(null);
const clearArmed = ref(false);

const canEdit = computed(() => source.value === "api" && !loading.value && !busyAction.value);
const canClearWorkspaceKey = computed(() => metadata.value.source === "workspace" && metadata.value.apiKeyConfigured);
const savedVerification = computed(() => verificationStatusMeta(metadata.value.verificationStatus));
const modelAvailability = computed(() => {
  if (draft.analysisEngine === "rule-engine") {
    return { label: "不会调用", tone: "neutral", detail: "已选择规则引擎" };
  }
  if (!metadata.value.apiKeyConfigured && !apiKeyDraft.value.trim()) {
    return { label: "不可调用", tone: "error", detail: "尚未配置 API Key" };
  }
  if (apiKeyDraft.value.trim()) {
    return { label: "待保存", tone: "warning", detail: "草稿 Key 尚未生效" };
  }
  if (metadata.value.verificationStatus === "verified") {
    return { label: "可调用", tone: "success", detail: "连接已验证" };
  }
  return { label: "待验证", tone: "warning", detail: "建议先验证连接" };
});
const latestVerification = computed(() => verificationResultMeta(verificationResult.value || {}));
const verificationCard = computed(() => {
  if (verificationResult.value) return latestVerification.value;
  if (metadata.value.verificationStatus === "verified") {
    return {
      tone: "success",
      title: "已保存配置验证通过",
      detail: `${metadata.value.model || "OpenAI"} · ${metadata.value.lastVerifyLatencyMs || 0} ms`,
    };
  }
  if (metadata.value.verificationStatus === "failed") {
    return {
      tone: "error",
      title: "已保存配置验证失败",
      detail: metadata.value.lastVerifyError || "请检查配置后重试",
    };
  }
  return {
    tone: "neutral",
    title: "尚未验证当前草稿",
    detail: "验证只测试当前输入，不会修改已保存配置。",
  };
});
const keyStateLabel = computed(() => {
  if (!metadata.value.apiKeyConfigured) return "未配置";
  return metadata.value.apiKeyHint || "已安全保存";
});
const keyStateTone = computed(() => (metadata.value.apiKeyConfigured ? "success" : "neutral"));
const configurationIssueLabel = computed(() => {
  if (metadata.value.modelAvailable) return "当前配置可发起 OpenAI 请求";
  if (draft.analysisEngine === "rule-engine") return "当前按设置使用规则引擎";
  if (apiKeyDraft.value.trim()) return "新 Key 仅存在于草稿，保存后才对分析生效";
  if (metadata.value.configurationIssue === "openai api key is not configured") return "当前工作区尚未配置 API Key";
  return metadata.value.configurationIssue || "当前尚未具备模型调用条件";
});

function syncDraft(nextMetadata) {
  const normalized = normalizeAIServiceSettings(nextMetadata);
  metadata.value = normalized;
  draft.provider = normalized.provider;
  draft.baseUrl = normalized.baseUrl;
  draft.model = normalized.model;
  draft.timeoutSeconds = normalized.timeoutSeconds;
  draft.analysisEngine = normalized.analysisEngine;
  draft.fallbackPolicy = normalized.fallbackPolicy;
}

watch(
  [
    () => draft.provider,
    () => draft.baseUrl,
    () => draft.model,
    () => draft.timeoutSeconds,
    () => draft.analysisEngine,
    () => draft.fallbackPolicy,
    apiKeyDraft,
  ],
  () => {
    if (busyAction.value !== "verify" && busyAction.value !== "save") {
      verificationResult.value = null;
    }
  },
);

function setFeedback(tone, title, detail = "") {
  feedback.value = { tone, title, detail };
}

function formatDate(value) {
  if (!value) return "暂无记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return "暂无记录";
  return date.toLocaleString("zh-CN");
}

async function loadSettings() {
  loading.value = true;
  source.value = "loading";
  errorMessage.value = "";
  feedback.value = null;
  try {
    syncDraft(await fetchAIServiceSettings());
    source.value = "api";
  } catch (error) {
    source.value = "unavailable";
    errorMessage.value = error instanceof Error ? error.message : "AI 服务配置加载失败";
  } finally {
    loading.value = false;
  }
}

function currentPayload() {
  const validationError = validateAIServiceDraft(draft);
  if (validationError) {
    setFeedback("error", "配置未通过校验", validationError);
    return null;
  }
  return buildAIServiceDraftPayload(draft, apiKeyDraft.value);
}

async function verifyConnection() {
  const payload = currentPayload();
  if (!payload || source.value !== "api") return;
  busyAction.value = "verify";
  feedback.value = null;
  clearArmed.value = false;
  try {
    const result = await verifyAIServiceSettings(payload);
    verificationResult.value = result;
    const resultMeta = verificationResultMeta(result);
    setFeedback(resultMeta.tone, resultMeta.title, resultMeta.detail);
  } catch (error) {
    setFeedback("error", "验证请求失败", error instanceof Error ? error.message : "无法验证 OpenAI API");
  } finally {
    busyAction.value = "";
  }
}

async function saveSettings() {
  const payload = currentPayload();
  if (!payload || source.value !== "api") return;
  if (apiKeyDraft.value.trim() && !metadata.value.apiKeyPersistenceAvailable) {
    setFeedback("warning", "当前服务端不能保存新 Key", "当前运行实例没有可用的配置存储；草稿仍可用于连接验证。");
    return;
  }
  busyAction.value = "save";
  feedback.value = null;
  clearArmed.value = false;
  const verifyOnSave = verificationResult.value?.ok === true;
  try {
    const saved = await updateAIServiceSettings(payload, { verifyOnSave });
    syncDraft(saved);
    apiKeyDraft.value = "";
    showAPIKey.value = false;
    verificationResult.value = null;
    setFeedback(
      "success",
      "配置已保存",
      verifyOnSave
        ? "配置和连接验证结果已保存；下一次 AI 分析会按当前工作区读取此配置。"
        : "下一次 AI 分析会按当前工作区读取此配置；保存不会把 Key 返回浏览器。",
    );
  } catch (error) {
    setFeedback("error", "配置保存失败", error instanceof Error ? error.message : "请检查服务端配置");
  } finally {
    busyAction.value = "";
  }
}

async function clearWorkspaceKey() {
  if (!canClearWorkspaceKey.value || source.value !== "api") return;
  if (!clearArmed.value) {
    clearArmed.value = true;
    setFeedback("warning", "再次点击确认清除", "只清除当前工作区保存的 Key；如果服务器环境变量有 Key，系统会自动回退使用。");
    return;
  }
  busyAction.value = "clear";
  try {
    syncDraft(await clearAIServiceSettingsKey());
    apiKeyDraft.value = "";
    verificationResult.value = null;
    clearArmed.value = false;
    setFeedback("success", "工作区 Key 已清除", `当前有效来源：${aiServiceSourceLabel(metadata.value.source)}`);
  } catch (error) {
    setFeedback("error", "清除失败", error instanceof Error ? error.message : "无法清除当前工作区 Key");
  } finally {
    busyAction.value = "";
  }
}

onMounted(loadSettings);
</script>

<template>
  <section class="ai-service-settings-panel" aria-labelledby="ai-service-settings-title">
    <header class="ai-service-settings-head">
      <div class="ai-service-settings-title">
        <span class="ai-service-icon" aria-hidden="true"><Bot :size="19" /></span>
        <div>
          <span class="panel-eyebrow">模型服务</span>
          <h2 id="ai-service-settings-title">OpenAI 服务配置</h2>
          <p>按当前运营身份独立保存。API Key 仅提交到服务端存储，不进入浏览器缓存。</p>
        </div>
      </div>
      <span class="ai-service-api-state" :data-tone="source === 'api' ? 'success' : source === 'unavailable' ? 'error' : 'neutral'">
        <span class="ai-service-api-dot" aria-hidden="true"></span>
        {{ source === "api" ? "服务端在线" : source === "unavailable" ? "API 不可用" : "读取中" }}
      </span>
    </header>

    <div v-if="!loading && source === 'api'" class="ai-service-owner-line">
      <span>当前配置身份</span>
      <strong>{{ metadata.ownerId || "未解析" }}</strong>
      <span>来源 {{ aiServiceSourceLabel(metadata.source) }}</span>
      <span class="ai-service-owner-state" :data-tone="metadata.modelAvailable ? 'success' : 'warning'">{{ configurationIssueLabel }}</span>
    </div>

    <div v-if="loading" class="ai-service-empty-state">
      <LoaderCircle class="ai-service-spinner" :size="19" aria-hidden="true" />
      <div><strong>正在读取 AI 服务配置</strong><span>配置只从真实 Insight API 获取。</span></div>
    </div>

    <div v-else-if="source === 'unavailable'" class="ai-service-empty-state" data-tone="error">
      <CircleAlert :size="19" aria-hidden="true" />
      <div><strong>暂时不能管理 AI 服务</strong><span>{{ errorMessage || "服务端配置接口不可用" }}</span></div>
      <button class="tool-button secondary" type="button" @click="loadSettings">重新读取</button>
    </div>

    <template v-else>
      <div class="ai-service-summary" aria-label="当前 AI 服务状态">
        <div class="ai-service-summary-item">
          <span>提供方</span><strong>OpenAI</strong><small>Responses API</small>
        </div>
        <div class="ai-service-summary-item">
          <span>有效来源</span><strong>{{ aiServiceSourceLabel(metadata.source) }}</strong><small>按 HubUserID 隔离</small>
        </div>
        <div class="ai-service-summary-item">
          <span>执行引擎</span><strong>{{ draft.analysisEngine === "openai" ? "OpenAI" : "规则引擎" }}</strong><small>{{ draft.analysisEngine === "openai" ? "优先调用模型" : "不调用模型" }}</small>
        </div>
        <div class="ai-service-summary-item">
          <span>模型可用性</span><strong class="ai-service-tone-text" :data-tone="modelAvailability.tone">{{ modelAvailability.label }}</strong><small>{{ modelAvailability.detail }}</small>
        </div>
        <div class="ai-service-summary-item">
          <span>保存状态</span>
          <strong class="ai-service-tone-text" :data-tone="savedVerification.tone">{{ savedVerification.label }}</strong>
          <small>{{ formatDate(metadata.lastVerifiedAt || metadata.updatedAt) }}</small>
        </div>
      </div>

      <div class="ai-service-body">
        <div class="ai-service-form-section">
          <div class="ai-service-section-head">
            <div><span class="panel-eyebrow">连接参数</span><h3>请求路由与模型</h3></div>
            <span class="ai-service-section-index">01</span>
          </div>

          <div class="ai-service-form-grid">
            <label class="ai-service-field">
              <span><strong>服务提供方</strong><small>当前版本固定</small></span>
              <select v-model="draft.provider" disabled aria-label="服务提供方">
                <option value="openai">OpenAI</option>
              </select>
            </label>
            <label class="ai-service-field">
              <span><strong>请求超时</strong><small>5–300 秒</small></span>
              <div class="ai-service-input-suffix">
                <input v-model.number="draft.timeoutSeconds" type="number" min="5" max="300" step="1" :disabled="!canEdit" />
                <span>秒</span>
              </div>
            </label>
            <label class="ai-service-field ai-service-field-wide">
              <span><strong>Base URL</strong><small>默认使用 OpenAI 官方地址</small></span>
              <input v-model.trim="draft.baseUrl" type="url" inputmode="url" autocomplete="url" :disabled="!canEdit" spellcheck="false" />
              <small class="ai-service-field-hint">仅在使用企业网关或兼容代理时修改，系统会在此地址后调用 /responses。</small>
            </label>
            <label class="ai-service-field ai-service-field-wide">
              <span><strong>模型名称</strong><small>用于下一次分析</small></span>
              <input v-model.trim="draft.model" type="text" autocomplete="off" :disabled="!canEdit" spellcheck="false" />
            </label>
          </div>

          <div class="ai-service-execution-section">
            <div class="ai-service-section-head">
              <div><span class="panel-eyebrow">执行策略</span><h3>分析如何运行</h3></div>
              <span class="ai-service-section-index">02</span>
            </div>
            <div class="ai-service-form-grid">
              <label class="ai-service-field">
                <span><strong>分析引擎</strong><small>重新运行分析时生效</small></span>
                <select v-model="draft.analysisEngine" :disabled="!canEdit" aria-label="分析引擎">
                  <option value="openai">OpenAI 大模型</option>
                  <option value="rule-engine">规则引擎（不调用模型）</option>
                </select>
              </label>
              <label class="ai-service-field">
                <span><strong>模型失败策略</strong><small>请求失败或响应无效时</small></span>
                <select v-model="draft.fallbackPolicy" :disabled="!canEdit" aria-label="模型失败策略">
                  <option value="rule-engine">允许规则引擎降级</option>
                  <option value="error">直接失败并保留原结果</option>
                </select>
              </label>
            </div>
            <div class="ai-service-strategy-note" :data-tone="modelAvailability.tone">
              <Bot :size="16" aria-hidden="true" />
              <p v-if="draft.analysisEngine === 'openai' && !metadata.apiKeyConfigured && !apiKeyDraft.trim()">
                <strong>已选择 OpenAI，但当前不会发起请求</strong>
                <span>当前工作区没有 API Key。输入并保存 Key、验证连接后，重新运行分析才会调用大模型。</span>
              </p>
              <p v-else-if="draft.analysisEngine === 'openai'">
                <strong>当前会先调用 OpenAI</strong>
                <span>{{ draft.fallbackPolicy === "error" ? "模型不可用时停止流程，不生成规则替代结果。" : "模型不可用时明确标记原因，并用规则引擎生成可复核结果。" }}</span>
              </p>
              <p v-else>
                <strong>当前不会调用大模型</strong>
                <span>重新运行只使用本地证据加权规则；切换为 OpenAI 并保存后才会请求服务端模型。</span>
              </p>
            </div>
            <p class="ai-service-field-hint ai-service-runtime-hint">本次分析会读取上方身份对应的已保存配置；没有 Key、身份不一致或 API 未授权时，系统会明确标记未调用/失败原因，不把规则结果伪装成模型结果。</p>
          </div>
        </div>

        <aside class="ai-service-secret-section">
          <div class="ai-service-section-head">
            <div><span class="panel-eyebrow">访问凭证</span><h3>API Key</h3></div>
            <span class="ai-service-section-index">03</span>
          </div>

          <div class="ai-service-key-status" :data-tone="keyStateTone">
            <KeyRound :size="17" aria-hidden="true" />
            <div><strong>{{ metadata.apiKeyConfigured ? "已有可用密钥" : "尚未配置密钥" }}</strong><span>{{ metadata.apiKeyConfigured ? keyStateLabel : "输入 Key 后可先验证，再保存到当前工作区。" }}</span></div>
          </div>

          <label class="ai-service-field">
            <span><strong>新 API Key</strong><small>留空表示保留现有 Key</small></span>
            <div class="ai-service-secret-input">
              <input
                v-model="apiKeyDraft"
                :type="showAPIKey ? 'text' : 'password'"
                autocomplete="new-password"
                placeholder="输入新 Key，页面不会回填旧值"
                :disabled="!canEdit"
                spellcheck="false"
              />
              <button type="button" :aria-label="showAPIKey ? '隐藏 API Key' : '显示 API Key'" :disabled="!canEdit" @click="showAPIKey = !showAPIKey">
                <EyeOff v-if="showAPIKey" :size="16" aria-hidden="true" />
                <Eye v-else :size="16" aria-hidden="true" />
              </button>
            </div>
          </label>

          <div class="ai-service-security-note">
            <ShieldCheck :size="17" aria-hidden="true" />
            <p>
              <strong>{{ metadata.apiKeyPersistenceAvailable ? "服务端存储已启用" : "尚未启用 Key 持久化" }}</strong>
              <span>{{ metadata.apiKeyPersistenceAvailable ? "保存到本机 JSON 或已配置的数据库；验证请求不落库，Key 不会返回浏览器。" : "可验证当前草稿，但当前运行实例没有可用的配置存储。" }}</span>
            </p>
          </div>

          <button
            class="ai-service-clear-button"
            :class="{ armed: clearArmed }"
            type="button"
            :disabled="!canEdit || !canClearWorkspaceKey"
            @click="clearWorkspaceKey"
          >
            <Trash2 :size="15" aria-hidden="true" />
            {{ busyAction === "clear" ? "清除中…" : clearArmed ? "确认清除工作区 Key" : "清除工作区 Key" }}
          </button>
        </aside>
      </div>

      <div class="ai-service-footer">
        <div class="ai-service-verification-card" :data-tone="verificationCard.tone">
          <CheckCircle2 v-if="verificationCard.tone === 'success'" :size="18" aria-hidden="true" />
          <CircleAlert v-else :size="18" aria-hidden="true" />
          <div>
            <strong>{{ verificationCard.title }}</strong>
            <span>{{ verificationCard.detail }}</span>
          </div>
        </div>
        <div class="ai-service-actions">
          <button class="tool-button secondary" type="button" :disabled="!canEdit" @click="verifyConnection">
            <LoaderCircle v-if="busyAction === 'verify'" class="ai-service-spinner" :size="15" aria-hidden="true" />
            <TestTube2 v-else :size="15" aria-hidden="true" />
            {{ busyAction === "verify" ? "验证中…" : "验证连接" }}
          </button>
          <button class="tool-button primary" type="button" :disabled="!canEdit" @click="saveSettings">
            <LoaderCircle v-if="busyAction === 'save'" class="ai-service-spinner" :size="15" aria-hidden="true" />
            <Save v-else :size="15" aria-hidden="true" />
            {{ busyAction === "save" ? "保存中…" : "保存配置" }}
          </button>
        </div>
      </div>

      <div v-if="feedback" class="ai-service-feedback" :data-tone="feedback.tone" aria-live="polite" :role="feedback.tone === 'error' ? 'alert' : 'status'">
        <CircleAlert v-if="feedback.tone === 'error' || feedback.tone === 'warning'" :size="16" aria-hidden="true" />
        <CheckCircle2 v-else :size="16" aria-hidden="true" />
        <div><strong>{{ feedback.title }}</strong><span v-if="feedback.detail">{{ feedback.detail }}</span></div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.ai-service-settings-panel {
  grid-column: 1 / -1;
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--analytics-line);
  border-radius: 7px;
  background: var(--analytics-surface);
  box-shadow: var(--analytics-shadow);
}

.ai-service-settings-head,
.ai-service-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 20px;
}

.ai-service-settings-head { align-items: flex-start; border-bottom: 1px solid var(--analytics-line-soft); }
.ai-service-owner-line { display: flex; align-items: center; flex-wrap: wrap; gap: 8px 12px; min-height: 38px; padding: 8px 20px; border-bottom: 1px solid var(--analytics-line-soft); color: var(--analytics-muted); background: #fbfcfe; font-size: .68rem; }
.ai-service-owner-line > span:first-child { color: var(--analytics-subtle); font-weight: 750; text-transform: uppercase; letter-spacing: .04em; }
.ai-service-owner-line strong { color: var(--analytics-ink); font-family: "Fira Code", Consolas, monospace; font-size: .7rem; font-weight: 650; }
.ai-service-owner-state { margin-left: auto; color: #8b611f; }
.ai-service-owner-state[data-tone="success"] { color: #267451; }
.ai-service-settings-title { display: flex; align-items: flex-start; gap: 12px; min-width: 0; }
.ai-service-icon { display: grid; flex: 0 0 36px; width: 36px; height: 36px; place-items: center; border: 1px solid #cbdaf5; border-radius: 6px; color: var(--analytics-blue); background: #edf3ff; }
.ai-service-settings-head h2,
.ai-service-section-head h3 { margin: 4px 0 5px; color: var(--analytics-ink); font-size: 1.12rem; font-weight: 750; }
.ai-service-settings-head p { max-width: 720px; margin: 0; color: var(--analytics-muted); font-size: .8rem; line-height: 1.55; }

.ai-service-api-state,
.ai-service-tone-text { display: inline-flex; align-items: center; gap: 6px; color: var(--analytics-muted); }
.ai-service-api-state { min-height: 25px; padding: 0 8px; border: 1px solid var(--analytics-line); border-radius: 4px; background: var(--analytics-surface-soft); font-size: .68rem; font-weight: 700; white-space: nowrap; }
.ai-service-api-dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
[data-tone="success"] { --state-color: #267451; --state-border: #cde5db; --state-bg: #f2fbf6; }
[data-tone="warning"] { --state-color: #8b611f; --state-border: #ead7ae; --state-bg: #fffaf0; }
[data-tone="error"] { --state-color: #a33a32; --state-border: #f0c6c2; --state-bg: #fff7f6; }
.ai-service-api-state[data-tone="success"], .ai-service-tone-text[data-tone="success"] { color: #267451; }
.ai-service-api-state[data-tone="error"], .ai-service-tone-text[data-tone="error"] { color: #a33a32; }
.ai-service-tone-text[data-tone="warning"] { color: #8b611f; }

.ai-service-summary { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); border-bottom: 1px solid var(--analytics-line-soft); background: #f8fafc; }
.ai-service-summary-item { display: grid; gap: 4px; min-width: 0; padding: 13px 16px; border-right: 1px solid var(--analytics-line-soft); }
.ai-service-summary-item:nth-child(5n), .ai-service-summary-item:last-child { border-right: 0; }
.ai-service-summary-item > span { color: var(--analytics-subtle); font-size: .64rem; font-weight: 750; letter-spacing: .05em; text-transform: uppercase; }
.ai-service-summary-item strong { overflow: hidden; color: var(--analytics-ink); font-family: "Fira Code", Consolas, monospace; font-size: .76rem; text-overflow: ellipsis; white-space: nowrap; }
.ai-service-summary-item small { color: var(--analytics-muted); font-size: .65rem; }

.ai-service-body { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(310px, .75fr); min-width: 0; }
.ai-service-form-section,
.ai-service-secret-section { display: grid; align-content: start; gap: 15px; min-width: 0; padding: 18px 20px 20px; }
.ai-service-secret-section { border-left: 1px solid var(--analytics-line-soft); background: #fbfcfe; }
.ai-service-execution-section { display: grid; gap: 13px; padding-top: 17px; border-top: 1px solid var(--analytics-line-soft); }
.ai-service-section-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.ai-service-section-head h3 { font-size: .94rem; }
.ai-service-section-index { color: #9babc0; font-family: "Fira Code", Consolas, monospace; font-size: .68rem; }
.ai-service-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 13px; }
.ai-service-field { display: grid; gap: 7px; min-width: 0; color: var(--analytics-ink); }
.ai-service-field-wide { grid-column: 1 / -1; }
.ai-service-field > span { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; font-size: .75rem; }
.ai-service-field > span small,
.ai-service-field-hint { color: var(--analytics-subtle); font-size: .65rem; font-weight: 500; }
.ai-service-field input,
.ai-service-field select { width: 100%; min-width: 0; min-height: 38px; padding: 0 10px; border: 1px solid #d6dee8; border-radius: 5px; outline: 0; color: #26374e; background: #fbfcfe; font: 500 .75rem/1.4 "Fira Code", Consolas, "Microsoft YaHei", monospace; transition: border-color .16s ease, box-shadow .16s ease; }
.ai-service-field input:focus,
.ai-service-field select:focus { border-color: var(--analytics-blue); box-shadow: 0 0 0 3px rgba(47, 111, 237, .1); }
.ai-service-field input:disabled,
.ai-service-field select:disabled { color: #7c8999; background: #f0f3f6; cursor: not-allowed; }
.ai-service-input-suffix,
.ai-service-secret-input { position: relative; display: flex; align-items: center; min-width: 0; }
.ai-service-input-suffix > span { position: absolute; right: 11px; color: var(--analytics-subtle); font-size: .67rem; pointer-events: none; }
.ai-service-input-suffix input { padding-right: 34px; }
.ai-service-secret-input input { padding-right: 40px; }
.ai-service-secret-input button { position: absolute; right: 3px; display: grid; width: 32px; height: 32px; place-items: center; border: 0; border-radius: 4px; color: #6c7d90; background: transparent; cursor: pointer; }
.ai-service-secret-input button:hover:not(:disabled) { color: var(--analytics-blue); background: #edf3ff; }
.ai-service-secret-input button:focus-visible { outline: 2px solid var(--analytics-blue); outline-offset: 1px; }

.ai-service-key-status,
.ai-service-security-note,
.ai-service-verification-card,
.ai-service-feedback { display: flex; align-items: flex-start; gap: 9px; border: 1px solid var(--state-border, var(--analytics-line)); border-radius: 5px; color: var(--state-color, var(--analytics-muted)); background: var(--state-bg, var(--analytics-surface-soft)); }
.ai-service-key-status { padding: 10px 11px; }
.ai-service-key-status > div,
.ai-service-verification-card > div,
.ai-service-feedback > div { display: grid; gap: 3px; min-width: 0; }
.ai-service-key-status strong,
.ai-service-verification-card strong,
.ai-service-feedback strong { color: inherit; font-size: .74rem; }
.ai-service-key-status span,
.ai-service-verification-card span,
.ai-service-feedback span { color: inherit; font-size: .67rem; line-height: 1.45; opacity: .9; }
.ai-service-security-note { padding: 10px 11px; border-color: #cfe0f2; color: #536a83; background: #f7fbff; }
.ai-service-strategy-note { display: flex; align-items: flex-start; gap: 9px; padding: 10px 11px; border: 1px solid #d7e3f2; border-radius: 5px; color: #49637f; background: #f7fbff; }
.ai-service-strategy-note p { display: grid; gap: 3px; margin: 0; }
.ai-service-strategy-note strong { color: #2f5277; font-size: .72rem; }
.ai-service-strategy-note span { color: #60748b; font-size: .66rem; line-height: 1.5; }
.ai-service-strategy-note[data-tone="error"] { border-color: #f0c6c2; color: #8f3f39; background: #fff7f6; }
.ai-service-strategy-note[data-tone="error"] strong { color: #8f3f39; }
.ai-service-strategy-note[data-tone="warning"] { border-color: #ead7ae; color: #7e602c; background: #fffaf0; }
.ai-service-strategy-note[data-tone="warning"] strong { color: #745321; }
.ai-service-runtime-hint { display: block; margin: -3px 0 0; line-height: 1.45; }
.ai-service-security-note p { display: grid; gap: 3px; margin: 0; }
.ai-service-security-note strong { color: #355270; font-size: .72rem; }
.ai-service-security-note span { font-size: .66rem; line-height: 1.5; }
.ai-service-clear-button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; min-height: 36px; padding: 0 12px; border: 1px solid #d8e0e8; border-radius: 5px; color: #6d7785; background: #fff; font-size: .72rem; font-weight: 700; cursor: pointer; }
.ai-service-clear-button:hover:not(:disabled) { border-color: #e5b4af; color: #a33a32; background: #fff7f6; }
.ai-service-clear-button.armed { border-color: #d98d85; color: #963229; background: #fff2f0; }
.ai-service-clear-button:disabled { opacity: .5; cursor: not-allowed; }

.ai-service-footer { border-top: 1px solid var(--analytics-line-soft); background: #fbfcfe; }
.ai-service-verification-card { flex: 1 1 auto; max-width: 650px; padding: 9px 11px; }
.ai-service-actions { display: flex; align-items: center; gap: 9px; flex: 0 0 auto; }
.ai-service-actions .tool-button { display: inline-flex; align-items: center; gap: 6px; min-height: 36px; }
.ai-service-feedback { margin: 0 20px 18px; padding: 10px 11px; }
.ai-service-empty-state { display: flex; align-items: center; gap: 11px; padding: 26px 20px; color: var(--analytics-muted); }
.ai-service-empty-state[data-tone="error"] { color: #a33a32; }
.ai-service-empty-state > div { display: grid; gap: 3px; flex: 1 1 auto; }
.ai-service-empty-state strong { color: var(--analytics-ink); font-size: .8rem; }
.ai-service-empty-state span { font-size: .72rem; }
.ai-service-spinner { animation: ai-service-spin .8s linear infinite; }
@keyframes ai-service-spin { to { transform: rotate(360deg); } }

@media (max-width: 980px) {
  .ai-service-summary { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .ai-service-summary-item:nth-child(3n) { border-right: 0; }
  .ai-service-summary-item:nth-child(-n + 2) { border-bottom: 1px solid var(--analytics-line-soft); }
  .ai-service-body { grid-template-columns: 1fr; }
  .ai-service-secret-section { border-top: 1px solid var(--analytics-line-soft); border-left: 0; }
}

@media (max-width: 620px) {
  .ai-service-settings-head,
  .ai-service-footer { align-items: flex-start; flex-direction: column; }
  .ai-service-summary,
  .ai-service-form-grid { grid-template-columns: 1fr; }
  .ai-service-summary-item { border-right: 0; border-bottom: 1px solid var(--analytics-line-soft); }
  .ai-service-summary-item:last-child { border-bottom: 0; }
  .ai-service-field-wide { grid-column: auto; }
  .ai-service-actions { width: 100%; flex-wrap: wrap; }
  .ai-service-actions .tool-button { flex: 1 1 150px; justify-content: center; }
}

@media (prefers-reduced-motion: reduce) {
  .ai-service-spinner { animation: none; }
}
</style>

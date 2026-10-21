<script setup>
import { computed, onMounted, ref } from "vue";
import { Check, RotateCcw, Save } from "lucide-vue-next";
import {
  fetchPromptSettings,
  resetPromptSetting,
  updatePromptSetting,
} from "../lib/api.js";

const profiles = ref([]);
const selectedKey = ref("");
const draftSystemPrompt = ref("");
const draftUserPrompt = ref("");
const draftEnabled = ref(false);
const loading = ref(true);
const busy = ref(false);
const source = ref("loading");
const errorMessage = ref("");
const feedback = ref("");
const feedbackTone = ref("success");

const selectedProfile = computed(
  () => profiles.value.find((profile) => profile.key === selectedKey.value) || null,
);
const canEdit = computed(() => source.value === "api" && !loading.value && !busy.value);
const draftSystemLength = computed(() => draftSystemPrompt.value.length);
const draftUserLength = computed(() => draftUserPrompt.value.length);
const profileOrder = [
  "content-analysis",
  "topic-trends",
  "account-performance",
  "comment-insights",
];

function syncDraft(profile = selectedProfile.value) {
  if (!profile) {
    draftSystemPrompt.value = "";
    draftUserPrompt.value = "";
    draftEnabled.value = false;
    return;
  }
  draftSystemPrompt.value = profile.systemPrompt || "";
  draftUserPrompt.value = profile.userPromptTemplate || "";
  draftEnabled.value = profile.enabled !== false;
}

function selectProfile(key) {
  selectedKey.value = key;
  feedback.value = "";
  syncDraft(profiles.value.find((profile) => profile.key === key));
}

function applyPayload(payload, keepKey = selectedKey.value) {
  const items = Array.isArray(payload?.items) ? payload.items : [];
  profiles.value = items.filter(profile => profileOrder.includes(profile.key)).sort(
    (left, right) => profileOrder.indexOf(left.key) - profileOrder.indexOf(right.key),
  );
  const nextKey = profiles.value.some((profile) => profile.key === keepKey)
    ? keepKey
    : profiles.value.find((profile) => profile.integrationState === "active")?.key || profiles.value[0]?.key || "";
  selectedKey.value = nextKey;
  syncDraft(profiles.value.find((profile) => profile.key === nextKey));
}

async function loadProfiles() {
  loading.value = true;
  source.value = "loading";
  errorMessage.value = "";
  try {
    const payload = await fetchPromptSettings();
    applyPayload(payload);
    source.value = "api";
  } catch (error) {
    profiles.value = [];
    selectedKey.value = "";
    source.value = "unavailable";
    errorMessage.value = error instanceof Error ? error.message : "Prompt 配置加载失败";
  } finally {
    loading.value = false;
  }
}

function validateDraft() {
  if (!draftSystemPrompt.value.trim()) return "系统 Prompt 不能为空";
  if (!draftUserPrompt.value.trim()) return "用户 Prompt 模板不能为空";
  return "";
}

async function saveProfile() {
  const validationError = validateDraft();
  if (validationError) {
    feedbackTone.value = "error";
    feedback.value = validationError;
    return;
  }
  if (!selectedProfile.value || source.value !== "api") return;

  busy.value = true;
  feedback.value = "";
  try {
    const payload = await updatePromptSetting(selectedProfile.value.key, {
      systemPrompt: draftSystemPrompt.value,
      userPromptTemplate: draftUserPrompt.value,
      enabled: draftEnabled.value,
    });
    applyPayload(payload, selectedProfile.value.key);
    feedbackTone.value = "success";
    feedback.value = "已保存，下一次数据解释将使用新版本。";
  } catch (error) {
    feedbackTone.value = "error";
    feedback.value = error instanceof Error ? error.message : "Prompt 保存失败";
  } finally {
    busy.value = false;
  }
}

async function resetProfile() {
  if (!selectedProfile.value || source.value !== "api") return;
  busy.value = true;
  feedback.value = "";
  try {
    const key = selectedProfile.value.key;
    const payload = await resetPromptSetting(key);
    applyPayload(payload, key);
    feedbackTone.value = "success";
    feedback.value = "已恢复内置默认，并生成新的配置版本。";
  } catch (error) {
    feedbackTone.value = "error";
    feedback.value = error instanceof Error ? error.message : "Prompt 恢复失败";
  } finally {
    busy.value = false;
  }
}

function integrationLabel(profile) {
  return profile?.integrationState === "active" ? "已接入 OpenAI" : "尚未接入模型";
}

function integrationTone(profile) {
  return profile?.integrationState === "active" ? "active" : "pending";
}

function formatVariables(variables = []) {
  return variables.map((item) => `{{${item}}}`).join("、");
}

function formatUpdatedAt(value) {
  if (!value) return "内置默认";
  const date = new Date(value);
  if (Number.isNaN(date.getTime()) || date.getUTCFullYear() <= 1) return "内置默认";
  return date.toLocaleString("zh-CN");
}

onMounted(loadProfiles);
</script>

<template>
  <section class="prompt-settings-panel" aria-labelledby="prompt-settings-title">
    <header class="prompt-settings-head">
      <div>
        <span class="panel-eyebrow">AI 配置</span>
        <h2 id="prompt-settings-title">Prompt 管理</h2>
        <p>统一维护分析规则。配置由服务端按运营身份保存，浏览器不会保存 API Key。</p>
      </div>
      <div class="prompt-settings-head-meta">
        <span class="prompt-settings-source" :data-tone="source === 'api' ? 'active' : source === 'unavailable' ? 'error' : 'pending'">
          <span class="prompt-settings-source-dot" aria-hidden="true"></span>
          {{ source === "api" ? "服务端配置" : source === "unavailable" ? "API 不可用" : "读取中" }}
        </span>
        <span v-if="selectedProfile" class="prompt-settings-version">当前 {{ selectedProfile.version }}</span>
      </div>
    </header>

    <div v-if="loading" class="prompt-settings-state">
      <span class="prompt-settings-loader" aria-hidden="true"></span>
      <span>正在读取服务端 Prompt 配置…</span>
    </div>

    <div v-else-if="source === 'unavailable'" class="prompt-settings-state prompt-settings-state-error">
      <strong>暂时不能编辑 Prompt</strong>
      <p>系统设置不会把配置写入浏览器本地。请连接真实 Insight API 后重试。</p>
      <code>{{ errorMessage || "服务端配置接口不可用" }}</code>
      <button class="tool-button secondary" type="button" @click="loadProfiles">重新读取</button>
    </div>

    <div v-else class="prompt-settings-editor">
      <nav class="prompt-profile-list" aria-label="Prompt 类型">
        <button
          v-for="profile in profiles"
          :key="profile.key"
          class="prompt-profile-item"
          :class="{ active: selectedKey === profile.key }"
          type="button"
          @click="selectProfile(profile.key)"
        >
          <span class="prompt-profile-index">{{ String(profiles.indexOf(profile) + 1).padStart(2, "0") }}</span>
          <span class="prompt-profile-copy">
            <strong>{{ profile.label }}</strong>
            <small>{{ profile.description }}</small>
          </span>
          <span class="prompt-state-badge" :data-tone="integrationTone(profile)">{{ integrationLabel(profile) }}</span>
        </button>
      </nav>

      <div v-if="selectedProfile" class="prompt-editor-main">
        <div class="prompt-editor-title-row">
          <div>
            <span class="panel-eyebrow">{{ selectedProfile.key }}</span>
            <h3>{{ selectedProfile.label }}</h3>
            <p>{{ selectedProfile.description }}</p>
          </div>
          <div class="prompt-editor-status">
            <span class="prompt-state-badge" :data-tone="integrationTone(selectedProfile)">{{ integrationLabel(selectedProfile) }}</span>
            <span class="prompt-editor-updated">版本 {{ selectedProfile.version }} · {{ formatUpdatedAt(selectedProfile.updatedAt) }}</span>
          </div>
        </div>

        <div class="prompt-editor-notice" :data-tone="integrationTone(selectedProfile)">
          <span class="prompt-editor-notice-mark">{{ selectedProfile.integrationState === "active" ? "LIVE" : "PENDING" }}</span>
          <span>{{ selectedProfile.integrationState === "active" ? "下一次数据解释会读取此配置。模型密钥仍只由服务端环境管理。" : "配置可以先保存，当前分析接口尚未接入此 Prompt，不会虚假显示已调用模型。" }}</span>
        </div>

        <div class="prompt-editor-fields">
          <label class="prompt-editor-field">
            <span class="prompt-editor-label-row"><strong>系统 Prompt</strong><small>{{ draftSystemLength.toLocaleString() }} 字符</small></span>
            <textarea v-model="draftSystemPrompt" :disabled="!canEdit" rows="9" spellcheck="false"></textarea>
            <small class="prompt-editor-field-hint">定义角色、事实边界和输出要求。</small>
          </label>
          <label class="prompt-editor-field">
            <span class="prompt-editor-label-row"><strong>用户 Prompt 模板</strong><small>{{ draftUserLength.toLocaleString() }} 字符</small></span>
            <textarea v-model="draftUserPrompt" :disabled="!canEdit" rows="9" spellcheck="false"></textarea>
            <small class="prompt-editor-field-hint">只可使用受控变量：{{ formatVariables(selectedProfile.variables) || "无" }}</small>
          </label>
        </div>

        <div class="prompt-editor-footer">
          <label class="prompt-enabled-toggle">
            <input v-model="draftEnabled" type="checkbox" :disabled="!canEdit" />
            <span><strong>允许此 Prompt 生效</strong><small>关闭后将由服务端返回可见降级状态</small></span>
          </label>
          <div class="prompt-editor-actions">
            <button class="tool-button secondary" type="button" :disabled="!canEdit" @click="resetProfile">
              <RotateCcw :size="15" aria-hidden="true" />
              恢复默认
            </button>
            <button class="tool-button primary" type="button" :disabled="!canEdit" @click="saveProfile">
              <Save :size="15" aria-hidden="true" />
              {{ busy ? "保存中…" : "保存 Prompt" }}
            </button>
          </div>
        </div>

        <div v-if="feedback" class="prompt-editor-feedback" :data-tone="feedbackTone">
          <Check v-if="feedbackTone === 'success'" :size="15" aria-hidden="true" />
          <span>{{ feedback }}</span>
        </div>
      </div>
    </div>
  </section>
</template>

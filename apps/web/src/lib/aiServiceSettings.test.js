import { test } from "node:test";
import assert from "node:assert/strict";
import {
  aiServiceSourceLabel,
  buildAIServiceDraftPayload,
  normalizeAIServiceSettings,
  validateAIServiceDraft,
  verificationResultMeta,
  verificationStatusMeta,
} from "./aiServiceSettings.js";

test("AI service metadata normalization never carries a plaintext key", () => {
  const result = normalizeAIServiceSettings({
    provider: " OpenAI ",
    ownerId: " owner-a ",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-5.6",
    timeoutSeconds: "60",
    source: "workspace",
    apiKeyConfigured: true,
    apiKeyHint: "****1234",
    apiKeyPersistenceAvailable: true,
    apiKey: "must-not-survive",
  });

  assert.equal(result.provider, "openai");
  assert.equal(result.ownerId, "owner-a");
  assert.equal(result.timeoutSeconds, 60);
  assert.equal(result.apiKeyHint, "****1234");
  assert.equal(result.apiKeyPersistenceAvailable, true);
  assert.equal("apiKey" in result, false);
});

test("AI service draft payload only includes a newly entered key", () => {
  const draft = { provider: "openai", baseUrl: "https://api.openai.com/v1/", model: " gpt-5.6 ", timeoutSeconds: 45 };
  assert.deepEqual(buildAIServiceDraftPayload(draft), {
    provider: "openai",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-5.6",
    timeoutSeconds: 45,
  });
  assert.equal(buildAIServiceDraftPayload(draft, " sk-test ").apiKey, "sk-test");
});

test("AI service draft payload carries explicit analysis execution settings", () => {
  const draft = {
    provider: "openai",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-5.6",
    timeoutSeconds: 45,
    analysisEngine: "rule-engine",
    fallbackPolicy: "error",
  };
  assert.deepEqual(buildAIServiceDraftPayload(draft), {
    provider: "openai",
    baseUrl: "https://api.openai.com/v1",
    model: "gpt-5.6",
    timeoutSeconds: 45,
    analysisEngine: "rule-engine",
    fallbackPolicy: "error",
  });
});

test("AI service draft validation enforces URL, model and timeout boundaries", () => {
  const valid = { provider: "openai", baseUrl: "https://api.openai.com/v1", model: "gpt-5.6", timeoutSeconds: 45 };
  assert.equal(validateAIServiceDraft(valid), "");
  assert.match(validateAIServiceDraft({ ...valid, baseUrl: "not-a-url" }), /Base URL/);
  assert.match(validateAIServiceDraft({ ...valid, model: "" }), /模型名称/);
  assert.match(validateAIServiceDraft({ ...valid, timeoutSeconds: 2 }), /5–300/);
});

test("AI service status helpers return professional source and result labels", () => {
  assert.equal(aiServiceSourceLabel("environment"), "服务器环境变量");
  assert.deepEqual(verificationStatusMeta("verified"), { label: "已验证", tone: "success" });
  assert.equal(verificationResultMeta({ ok: false, errorCode: "authentication" }).title, "API Key 无效或权限不足");
  assert.equal(verificationResultMeta({ ok: true, model: "gpt-5.6", latencyMs: 128 }).tone, "success");
});

import { test } from "node:test";
import assert from "node:assert/strict";
import {
  buildAIAnalysisEvidenceRefs,
  buildAIAnalysisPayload,
  buildAIAnalysisResultSections,
  buildAIAnalysisVariables,
  getAIAnalysisModuleLabel,
  getAIAnalysisStatusMeta,
  normalizeAIAnalysisResult,
  normalizeAIAnalysisRun,
} from "./aiAnalysis.js";

test("AI analysis status metadata distinguishes model success from non-result states", () => {
  assert.equal(getAIAnalysisStatusMeta("completed").hasResult, true);
  assert.equal(getAIAnalysisStatusMeta("completed").label, "模型完成");
  assert.equal(getAIAnalysisStatusMeta("fallback").hasResult, false);
  assert.equal(getAIAnalysisStatusMeta("failed").tone, "risk");
  assert.equal(getAIAnalysisStatusMeta("disabled").label, "未启用");
});

test("AI analysis result normalization keeps typed strings, lists and evidence separate", () => {
  const raw = {
    summary: "  结论  ",
    strengths: ["优势一", "", 42, "优势二"],
    evidenceRefs: ["video:1", "", 9],
    unexpected: "额外字段",
  };

  assert.deepEqual(normalizeAIAnalysisResult("content-analysis", raw), {
    summary: "结论",
    strengths: ["优势一", "优势二"],
    unexpected: "额外字段",
  });
  assert.deepEqual(buildAIAnalysisEvidenceRefs(raw), ["video:1"]);
  assert.deepEqual(buildAIAnalysisResultSections("content-analysis", raw), [
    { key: "summary", label: "摘要", type: "text", value: "结论" },
    { key: "strengths", label: "优势", type: "list", value: ["优势一", "优势二"] },
    { key: "unexpected", label: "unexpected", type: "text", value: "额外字段" },
  ]);
});

test("AI analysis run normalization suppresses results for fallback and exposes trace metadata", () => {
  const fallback = normalizeAIAnalysisRun("content-analysis", {
    module: "content-analysis",
    status: "fallback",
    result: { summary: "不应展示" },
    scopeKey: "videos",
    window: "24h",
    modelProvider: "openai",
    modelName: "gpt-5.6",
    durationMs: 31,
    inputFingerprint: "sha256:test",
    runAt: "2026-09-01T08:00:00Z",
  });

  assert.equal(fallback.result, null);
  assert.deepEqual(fallback.resultSections, []);
  assert.equal(fallback.statusMeta.label, "已降级");
  assert.equal(fallback.trace.find((item) => item.key === "model")?.value, "openai · gpt-5.6");
  assert.equal(fallback.trace.find((item) => item.key === "fingerprint")?.value, "sha256:test");
});

test("AI analysis variables follow the four data interpretation module input contracts", () => {
  const source = {
    data: { window: "24h" },
    video: { id: "video-1" },
    topic: { id: "topic-1" },
    analysis: { angle: "从证据切入" },
    account: { id: "account-1" },
    comments: [{ id: "comment-1" }],
    report: { headline: "日报" },
  };

  assert.deepEqual(buildAIAnalysisVariables("content-analysis", source), {
    data_json: source.data,
    video_json: source.video,
  });
  assert.deepEqual(buildAIAnalysisVariables("account-performance", source), {
    data_json: source.data,
    account_json: source.account,
  });
  assert.deepEqual(buildAIAnalysisVariables("comment-insights", source), {
    data_json: source.data,
    comments_json: source.comments,
  });
  assert.deepEqual(buildAIAnalysisPayload("topic-trends", { scopeKey: "topics", window: "72h", data: source.data }), {
    scopeKey: "topics",
    window: "72h",
    variables: { data_json: source.data },
  });
  assert.equal(getAIAnalysisModuleLabel("topic-trends"), "关键词样本解释");
});

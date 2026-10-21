import test from "node:test";
import assert from "node:assert/strict";
import * as api from "./api.js";

test("the frontend API exposes facts and monitoring without production or idea mutations", () => {
  for (const name of ["renderProduction", "rerenderProduction", "downloadReferenceAsset", "analyzeTopic", "saveTeamFollowUps", "fetchTopicRecommendations", "fetchAllTopicRecommendations", "fetchTodayTopicRecommendations", "createDailyReportSnapshot", "addManualTopic", "fetchDailyReports", "fetchDailyReport", "fetchDashboard", "fetchHotVideos", "fetchHotTopics", "fetchCommentInsights", "fetchOperatorRuns", "fetchAIAnalysisRun", "fetchAccountRecommendations", "fetchVideoRecommendations", "exportRawComments", "deleteDataCenterRecord", "restoreDataCenterRecord", "appendMockIngestBatch", "syncWXCommentExport", "importWXFeedList", "searchWXVideos", "importWXVideoSearch", "saveWXCommentExportBinding", "deleteSavedSourceBinding", "resetIngestSummary", "retryLastFailedIngest", "runOperatorCycle", "runOperatorLoop", "tickMonitoringScheduler"]) {
    assert.equal(name in api, false, name);
  }
  for (const name of ["fetchAccountVideos", "fetchVideoDetail", "fetchVideoComments", "fetchDataCenter", "createMonitoringTask", "runMonitoringTask", "retryMonitoringTask"]) assert.equal(typeof api[name], "function", name);
});

test("retired AI modules are rejected locally before making a request", async () => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = async () => { calls++; throw new Error("unexpected fetch"); };
  try {
    for (const module of ["topic-analysis", "topic-script", "daily-report"]) await assert.rejects(api.runAIAnalysis(module), /removed|retired|移除/i);
    for (const module of ["topic-analysis", "topic-script", "daily-report"]) {
      await assert.rejects(api.fetchAIAnalysisRuns(module), /removed|retired|移除/i);
      await assert.rejects(api.updatePromptSetting(module, {}), /removed|retired|移除/i);
      await assert.rejects(api.resetPromptSetting(module), /removed|retired|移除/i);
    }
    assert.equal(calls, 0);
  } finally { globalThis.fetch = originalFetch; }
});

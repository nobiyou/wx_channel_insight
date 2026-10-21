import test from "node:test";
import assert from "node:assert/strict";
import { OPERATIONS_NAVIGATION, parseOperationsRoute, buildOperationsHash, videoFactsQuery, monitoringTasksForScope } from "./operationsNavigation.js";

test("navigation offers the three core analysis tasks and supporting overview/settings", () => {
  assert.deepEqual(OPERATIONS_NAVIGATION.map((item) => item.key), ["dashboard", "accounts", "keywords", "videos", "agent", "settings"]);
});

test("only current routes are recognized without historical aliases", () => {
  for (const hash of ["#topic-lab", "#topic-analysis/example", "#topic-analysis?topic=one", "#production", "#watchlists?mode=topic-lab", "#comments", "#topics", "#watchlists", "#data-center", "#tasks", "#sources", "#account/old-id"]) {
    assert.equal(parseOperationsRoute(hash).view, "dashboard", hash);
  }
  assert.equal(parseOperationsRoute("#reports").view, "dashboard");
  assert.equal(parseOperationsRoute("#account?id=current-id").id, "current-id");
});

test("video return context includes exact keyword, filters and pagination", () => {
  const returnHash = buildOperationsHash("keywords", { keyword: "家居 & 收纳", account: "finder-account", page: 3, pageSize: 50, sort: "likes", from: "2026-10-01" });
  const detail = parseOperationsRoute(buildOperationsHash("video", { id: "real/id", return: returnHash }));
  assert.equal(detail.id, "real/id");
  assert.equal(detail.returnHash, returnHash);
  const list = parseOperationsRoute(detail.returnHash);
  assert.equal(list.params.keyword, "家居 & 收纳");
  assert.equal(list.params.page, "3");
  const query = videoFactsQuery(list.params, list.view);
  assert.equal(query.monitorKeyword, "家居 & 收纳");
  assert.equal(query.q, "");
  assert.equal(query.page, 3);
  assert.equal(query.account, "finder-account");
});

test("overview video details return to overview and reject unrelated destinations", () => {
  assert.equal(parseOperationsRoute(buildOperationsHash("video", { id: "real-video", return: "#dashboard" })).returnHash, "#dashboard");
  assert.equal(parseOperationsRoute(buildOperationsHash("video", { id: "real-video", return: "https://example.com" })).returnHash, "#videos");
});

test("keyword scope selects genuine keyword tasks and account scope excludes independent comment monitoring", () => {
  const tasks = [
    { id: "keyword", taskType: "legacy", scopeMode: "selected", targets: [{ source: "wx_video_search", targetKey: "收纳" }] },
    { id: "account", taskType: "video_list", targets: [{ source: "account", accountId: "a" }] },
    { id: "comments", taskType: "comment_collection", targets: [{ source: "account", accountId: "a" }] },
    { id: "system", system: true, scopeMode: "all_saved_bindings" },
  ];
  assert.deepEqual(monitoringTasksForScope(tasks, "keywords", "收纳").map((task) => task.id), ["keyword"]);
  assert.deepEqual(monitoringTasksForScope(tasks, "accounts").map((task) => task.id), ["account"]);
  assert.deepEqual(monitoringTasksForScope(tasks, "accounts", "a").map((task) => task.id), ["account"]);
  assert.deepEqual(monitoringTasksForScope(tasks, "accounts", "other").map((task) => task.id), []);
  assert.deepEqual(monitoringTasksForScope(tasks, "diagnostics").map((task) => task.id), ["keyword", "account", "comments", "system"]);
});

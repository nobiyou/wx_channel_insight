import { test } from "node:test"
import assert from "node:assert/strict"
import {
  canRetryMonitoringTask,
  DEFAULT_MONITORING_INTERVAL,
  formatMonitoringInterval,
  formatMonitoringTime,
  isMonitoringTaskRunning,
  monitoringRunMessage,
  monitoringSourceLabel,
  monitoringIntervalOptions,
  monitoringStatusTone,
  monitoringTargetLabel,
  monitoringTaskCollectsComments,
  monitoringTaskRunDetail,
  normalizeMonitoringIntervalForTask,
  normalizeMonitoringTarget,
  taskTargetCount,
} from "./monitoringTasks.js"

test("monitoring labels keep task status and source names user-facing", () => {
  assert.equal(monitoringSourceLabel("wx_feed_list"), "视频列表")
  assert.equal(monitoringSourceLabel("wx_video_search"), "关键词视频")
  assert.equal(monitoringStatusTone("failed"), "risk")
  assert.equal(monitoringStatusTone("enabled"), "good")
})

test("monitoring helpers format intervals, targets and retry state", () => {
  assert.equal(formatMonitoringInterval("15m"), "15 分钟")
  assert.equal(formatMonitoringInterval("1h30m"), "1 小时 30 分钟")
  assert.equal(formatMonitoringTime("", "暂无"), "暂无")
  assert.equal(
    monitoringTargetLabel({ displayName: "团队号", targetKey: "@teamfinder" }),
    "团队号",
  )
	assert.equal(taskTargetCount({ scopeMode: "all_saved_bindings" }), "全部已保存来源")
	assert.equal(taskTargetCount({ scopeMode: "all_accounts" }), "全部账号")
  assert.equal(taskTargetCount({ scopeMode: "selected", targets: [{ targetKey: "a" }] }), "1 个目标")
  assert.equal(canRetryMonitoringTask({ lastStatus: "partial" }), true)
  assert.equal(canRetryMonitoringTask({ lastStatus: "success" }), false)
  assert.equal(isMonitoringTaskRunning({ lastStatus: "running" }), true)
  assert.equal(isMonitoringTaskRunning({ lastStatus: "success" }), false)
  assert.equal(
    monitoringRunMessage("operator cycle is already running"),
    "已有监控任务正在执行，请等待本次运行完成后再操作",
  )
  assert.equal(monitoringRunMessage("具体失败原因"), "具体失败原因")
})

test("task cards show the current partial-run reason without reusing stale history", () => {
  const task = { lastStatus: "partial", lastError: "" }
  assert.equal(
    monitoringTaskRunDetail(task, {
      status: "partial",
      errors: [{ message: "本轮已达到评论采集安全上限 3 个视频，其余 6831 个将在后续运行继续采集" }],
    }),
    "本轮已达到评论采集安全上限 3 个视频，其余 6831 个将在后续运行继续采集",
  )
  assert.equal(monitoringTaskRunDetail(task, { status: "success", errors: [{ message: "旧原因" }] }), "")
  assert.equal(monitoringTaskRunDetail({ ...task, lastError: "任务错误" }, null), "任务错误")
})

test("comment monitoring enforces safe intervals and preserves account target fields", () => {
  const accountTarget = normalizeMonitoringTarget({
    source: "account",
    targetKey: "account-1",
    targetKind: "account",
    accountId: "account-1",
    commentMode: "incremental",
    displayName: "团队账号",
  })
  const task = { scopeMode: "selected", targets: [accountTarget] }
  assert.equal(accountTarget.accountId, "account-1")
  assert.equal(accountTarget.commentMode, "incremental")
  assert.equal(monitoringSourceLabel("account"), "重点账号")
  assert.equal(monitoringTaskCollectsComments(task), true)
  assert.equal(normalizeMonitoringIntervalForTask("5m", task), DEFAULT_MONITORING_INTERVAL)
  assert.deepEqual(monitoringIntervalOptions(task).map((option) => option.value), ["30m", "1h", "6h", "12h", "24h"])
  assert.equal(
    normalizeMonitoringIntervalForTask("5m", { scopeMode: "selected", targets: [{ source: "wx_feed_list" }] }),
    "5m",
  )
})

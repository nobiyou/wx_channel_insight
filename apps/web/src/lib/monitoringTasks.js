const SOURCE_LABELS = {
  account: "重点账号",
  wx_comment_export: "评论导出",
  wx_feed_list: "视频列表",
  wx_video_search: "关键词视频",
  wx_manual_share: "手动分享",
  wx_channel_comment_export: "评论导出",
  wx_channel_feed_list: "视频列表",
  wx_channel_video_search: "关键词视频",
};

export const DEFAULT_MONITORING_INTERVAL = "30m";
export const MONITORING_INTERVAL_OPTIONS = [
  { label: "每 1 分钟", value: "1m" },
  { label: "每 5 分钟", value: "5m" },
  { label: "每 15 分钟", value: "15m" },
  { label: "每 30 分钟", value: "30m" },
  { label: "每 1 小时", value: "1h" },
  { label: "每 6 小时", value: "6h" },
  { label: "每 12 小时", value: "12h" },
  { label: "每天", value: "24h" },
];

const TASK_STATUS_LABELS = {
  enabled: "已启用",
  paused: "已暂停",
  archived: "已归档",
};

const RUN_STATUS_LABELS = {
  running: "执行中",
  success: "成功",
  partial: "部分成功",
  failed: "失败",
  skipped: "已跳过",
  abandoned: "租约超时",
};

const TASK_TYPE_LABELS = {
  legacy: "数据采集",
  account_profile: "账号资料",
  video_list: "视频列表",
  comment_collection: "评论采集",
};

export function monitoringSourceLabel(source) {
  return SOURCE_LABELS[String(source || "").trim()] || "其他来源";
}

export function monitoringTaskStatusLabel(status) {
  return TASK_STATUS_LABELS[String(status || "").trim()] || "未知状态";
}

export function monitoringTaskTypeLabel(taskType) {
  return TASK_TYPE_LABELS[String(taskType || "legacy").trim()] || "数据采集";
}

export function monitoringTaskTypeTone(taskType) {
  switch (String(taskType || "legacy").trim()) {
    case "account_profile":
      return "profile";
    case "video_list":
      return "video";
    case "comment_collection":
      return "comment";
    default:
      return "legacy";
  }
}

export function monitoringRunStatusLabel(status) {
  return RUN_STATUS_LABELS[String(status || "").trim()] || "未知结果";
}

export function findMonitoringObjectTask(tasks, source, targetKey) {
  return (Array.isArray(tasks) ? tasks : []).find(task =>
    task.status !== "archived" && task.scopeMode === "selected" &&
    (!task.taskType || task.taskType === "legacy") && task.targets?.length === 1 &&
    task.targets[0].source === source && task.targets[0].targetKey === targetKey
  ) || null;
}

export function monitoringRunMessage(message, fallback = "") {
  const normalized = String(message || "").trim();
  if (!normalized) return fallback;
  if (
    normalized.includes("monitoring task run is already in progress") ||
    normalized.includes("operator cycle is already running")
  ) {
    return "已有监控任务正在执行，请等待本次运行完成后再操作";
  }
  return normalized;
}

export function monitoringTaskRunDetail(task, run) {
  const lastError = String(task?.lastError || "").trim();
  if (lastError) return monitoringRunMessage(lastError);
  if (!run || String(run.status || "").trim() !== String(task?.lastStatus || "").trim()) return "";
  const error = (Array.isArray(run.errors) ? run.errors : []).find((item) => String(item?.message || "").trim());
  return monitoringRunMessage(error?.message || run.message || "");
}

export function monitoringStatusTone(status) {
  switch (String(status || "").trim()) {
    case "enabled":
    case "success":
      return "good";
    case "paused":
    case "partial":
    case "running":
      return "warn";
    case "failed":
    case "abandoned":
      return "risk";
    default:
      return "neutral";
  }
}

export function formatMonitoringInterval(interval) {
  const value = String(interval || "").trim();
  const match = value.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/i);
  if (!value || !match || (!match[1] && !match[2] && !match[3])) {
    return value || "未设置周期";
  }
  const parts = [];
  if (match[1]) parts.push(`${Number(match[1])} 小时`);
  if (match[2]) parts.push(`${Number(match[2])} 分钟`);
  if (match[3]) parts.push(`${Number(match[3])} 秒`);
  return parts.join(" ");
}

export function formatMonitoringTime(value, placeholder = "未安排") {
  if (!value) return placeholder;
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return placeholder;
  return date.toLocaleString("zh-CN", { hour12: false });
}

export function monitoringTargetLabel(target) {
  const displayName = String(target?.displayName || "").trim();
  const label = String(target?.label || "").trim();
  const key = String(target?.targetKey || "").trim();
  return displayName || label || key || "未命名目标";
}

export function normalizeMonitoringTarget(target = {}) {
  return {
    source: String(target.source || "").trim(),
    targetKey: String(target.targetKey || "").trim(),
    targetKind: String(target.targetKind || "").trim(),
    accountId: String(target.accountId || "").trim(),
    commentMode: String(target.commentMode || "").trim(),
    label: String(target.label || "").trim(),
    displayName: String(target.displayName || "").trim(),
  };
}

export function monitoringTaskCollectsComments(task = {}) {
	if (task.taskType === "account_profile" || task.taskType === "video_list") return false;
	if (task.taskType === "comment_collection") return true;
  if (task.scopeMode === "all_saved_bindings") return true;
  return (Array.isArray(task.targets) ? task.targets : []).some((target) => {
    const normalized = normalizeMonitoringTarget(target);
    return normalized.targetKind === "account" || normalized.source === "account" || normalized.source === "wx_comment_export";
  });
}

export function monitoringIntervalOptions(task = {}) {
  if (!monitoringTaskCollectsComments(task)) return MONITORING_INTERVAL_OPTIONS;
  return MONITORING_INTERVAL_OPTIONS.filter((option) => intervalSeconds(option.value) >= 30 * 60);
}

export function normalizeMonitoringIntervalForTask(interval, task = {}) {
  const value = String(interval || "").trim() || DEFAULT_MONITORING_INTERVAL;
  if (monitoringTaskCollectsComments(task) && intervalSeconds(value) < 30 * 60) {
    return DEFAULT_MONITORING_INTERVAL;
  }
  return value;
}

function intervalSeconds(value) {
  const match = String(value || "").trim().match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/i);
  if (!match || (!match[1] && !match[2] && !match[3])) return 0;
  return Number(match[1] || 0) * 3600 + Number(match[2] || 0) * 60 + Number(match[3] || 0);
}

export function canRetryMonitoringTask(task) {
  return ["failed", "partial", "skipped", "abandoned"].includes(
    String(task?.lastStatus || "").trim(),
  );
}

export function isMonitoringTaskRunning(task) {
  return String(task?.lastStatus || "").trim() === "running";
}

export function taskTargetCount(task) {
	if (task?.scopeMode === "all_saved_bindings") return "全部已保存来源";
	if (task?.scopeMode === "all_accounts") return "全部账号";
	return `${Array.isArray(task?.targets) ? task.targets.length : 0} 个目标`;
}

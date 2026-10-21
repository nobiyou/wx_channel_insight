export function videoGrowthLabel(row = {}) {
  if (row.growthStatus === "metrics_unavailable") return "指标缺失，增幅不可用";
  if (row.growthStatus === "counter_correction") return "计数修正 / 异常";
  if (row.growthStatus === "zero_baseline") return "基线为 0，增幅不可用";
  if (row.growthStatus !== "available" || row.growthRate === null || row.growthRate === undefined || row.growthRate === "" || !Number.isFinite(Number(row.growthRate))) return "待积累历史";
  return `${Number(row.growthRate).toFixed(1)}%`;
}

const HOUR_MS = 60 * 60 * 1000;
const HOURLY_RANGE_LIMIT_MS = 48 * HOUR_MS;

export const VIDEO_METRIC_GRANULARITY_LABELS = Object.freeze({
  hour: "按小时",
  day: "按天",
});

function toObservedTime(value) {
  if (!value) return null;
  const time = new Date(value).getTime();
  return Number.isFinite(time) ? time : null;
}

function localBucketKey(value, granularity) {
  const date = new Date(value);
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  if (granularity === "day") return `${year}-${month}-${day}`;
  const hour = String(date.getHours()).padStart(2, "0");
  return `${year}-${month}-${day}-${hour}`;
}

export function resolveVideoMetricGranularity(points) {
  const timestamps = (Array.isArray(points) ? points : [])
    .map((point) => toObservedTime(point?.observedAt))
    .filter((value) => value !== null);

  if (timestamps.length < 2) return "hour";
  const range = Math.max(...timestamps) - Math.min(...timestamps);
  return range <= HOURLY_RANGE_LIMIT_MS ? "hour" : "day";
}

export function videoMetricGranularityLabel(granularity) {
  return VIDEO_METRIC_GRANULARITY_LABELS[granularity] || VIDEO_METRIC_GRANULARITY_LABELS.hour;
}

export function aggregateVideoMetricPoints(points, requestedGranularity = "auto") {
  const source = Array.isArray(points) ? points : [];
  const granularity =
    requestedGranularity === "hour" || requestedGranularity === "day"
      ? requestedGranularity
      : resolveVideoMetricGranularity(source);

  if (source.length < 2) {
    return {
      points: source.slice(),
      granularity,
      label: videoMetricGranularityLabel(granularity),
      rawCount: source.length,
    };
  }

  const entries = source.map((point, index) => ({
    point,
    index,
    observedTime: toObservedTime(point?.observedAt),
  }));

  // A timestamp is required for a reliable bucket. Keep the original series
  // intact if an upstream record is malformed instead of silently dropping it.
  if (entries.some((entry) => entry.observedTime === null)) {
    return {
      points: source.slice(),
      granularity,
      label: videoMetricGranularityLabel(granularity),
      rawCount: source.length,
    };
  }

  entries.sort((left, right) => left.observedTime - right.observedTime || left.index - right.index);
  const latestByBucket = new Map();
  for (const entry of entries) {
    latestByBucket.set(localBucketKey(entry.observedTime, granularity), entry);
  }

  const aggregated = [...latestByBucket.values()]
    .sort((left, right) => left.observedTime - right.observedTime)
    .map((entry) => entry.point);

  return {
    points: aggregated,
    granularity,
    label: videoMetricGranularityLabel(granularity),
    rawCount: source.length,
  };
}

import test from "node:test";
import assert from "node:assert/strict";
import {
  aggregateVideoMetricPoints,
  resolveVideoMetricGranularity,
} from "./videoMetricAggregation.js";

function point(observedAt, likes) {
  return { observedAt, likes };
}

test("uses hourly buckets for short histories and keeps the latest cumulative sample", () => {
  const points = [
    point("2026-08-14T09:05:00+08:00", 10),
    point("2026-08-14T09:45:00+08:00", 12),
    point("2026-08-14T10:05:00+08:00", 14),
    point("2026-08-14T11:15:00+08:00", 17),
  ];

  const result = aggregateVideoMetricPoints(points);

  assert.equal(result.granularity, "hour");
  assert.equal(result.label, "按小时");
  assert.equal(result.rawCount, 4);
  assert.deepEqual(result.points.map((item) => item.likes), [12, 14, 17]);
  assert.deepEqual(result.points.map((item) => item.observedAt), [
    "2026-08-14T09:45:00+08:00",
    "2026-08-14T10:05:00+08:00",
    "2026-08-14T11:15:00+08:00",
  ]);
});

test("switches to daily buckets for histories longer than two days", () => {
  const points = [
    point("2026-08-10T09:00:00+08:00", 10),
    point("2026-08-10T23:00:00+08:00", 12),
    point("2026-08-11T08:00:00+08:00", 14),
    point("2026-08-13T10:00:00+08:00", 19),
  ];

  const result = aggregateVideoMetricPoints(points);

  assert.equal(resolveVideoMetricGranularity(points), "day");
  assert.equal(result.granularity, "day");
  assert.deepEqual(result.points.map((item) => item.likes), [12, 14, 19]);
});

test("does not mutate or discard a series with an invalid observation timestamp", () => {
  const points = [
    point("not-a-date", 10),
    point("2026-08-14T10:00:00+08:00", 12),
  ];

  const result = aggregateVideoMetricPoints(points);

  assert.deepEqual(result.points, points);
  assert.notEqual(result.points, points);
});

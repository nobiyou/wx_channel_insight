import test from "node:test";
import assert from "node:assert/strict";
import { metricsAreStale, startAutoRefresh } from "./autoRefresh.js";

test("only missing or ten-minute-old observations require collection", () => {
  const now = Date.parse("2026-10-16T10:00:00+08:00");
  assert.equal(metricsAreStale(null, now), true);
  assert.equal(metricsAreStale("2026-10-16T09:50:00+08:00", now), true);
  assert.equal(metricsAreStale("2026-10-16T09:50:01+08:00", now), false);
  assert.equal(metricsAreStale("2026-10-16T10:01:00+08:00", now), false);
});

test("auto reads pause when hidden, recover after failure, and stop after disposal", async () => {
  let callback; let visible = false; let calls = 0; let failures = 0; let armed = 0;
  const loop = startAutoRefresh(async () => { calls++; if (calls === 1) throw new Error("offline"); }, {
    visible: () => visible, schedule: (fn) => { callback = fn; return ++armed; }, cancel: () => {}, onError: () => failures++,
  });
  await callback(); assert.equal(calls, 0);
  visible = true; await callback(); assert.equal(calls, 1); assert.equal(failures, 1);
  await callback(); assert.equal(calls, 2);
  loop.stop(); await callback(); assert.equal(calls, 2);
});

test("slow page requests never overlap and stopping an in-flight read does not rearm", async () => {
  let finish; let calls = 0; let arms = 0; let callback;
  const loop = startAutoRefresh(() => { calls++; return new Promise((resolve) => { finish = resolve; }); }, {
    visible: () => true, schedule: (fn) => { callback = fn; return ++arms; }, cancel: () => {},
  });
  const pending = callback(); await loop.refresh(); assert.equal(calls, 1);
  loop.stop(); finish(); await pending; assert.equal(arms, 1);
});

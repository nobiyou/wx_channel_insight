// Only read the active page. Hidden tabs and overlapping requests never start
// another poll, and a rejected read must not stop future updates.
export function metricsAreStale(observedAt, now = Date.now()) {
  const observed = Date.parse(observedAt || "");
  return !Number.isFinite(observed) || now - observed >= 10 * 60000;
}

export function startAutoRefresh(read, { interval = 30000, visible = () => document.visibilityState === "visible", schedule = setTimeout, cancel = clearTimeout, onError = () => {} } = {}) {
  let stopped = false;
  let running = false;
  let timer;
  const arm = () => { if (!stopped) timer = schedule(tick, interval); };
  async function tick() {
    if (stopped || running) return;
    cancel(timer);
    if (visible()) {
      running = true;
      try { await read(); } catch (error) { onError(error); } finally { running = false; }
    }
    arm();
  }
  arm();
  return { refresh: tick, stop() { stopped = true; cancel(timer); } };
}

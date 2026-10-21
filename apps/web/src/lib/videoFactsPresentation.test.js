import test from "node:test";
import assert from "node:assert/strict";
import { videoGrowthLabel } from "./videoFactsPresentation.js";

test("growth requires a server-confirmed historical baseline and retains unavailable status", () => {
  assert.equal(videoGrowthLabel({}), "待积累历史");
  assert.equal(videoGrowthLabel({ growthRate: 0 }), "待积累历史");
  assert.equal(videoGrowthLabel({ growthStatus: "available", growthRate: null }), "待积累历史");
  assert.equal(videoGrowthLabel({ growthStatus: "available", growthRate: 0 }), "0.0%");
  assert.equal(videoGrowthLabel({ growthStatus: "available", growthRate: 12.34 }), "12.3%");
  assert.equal(videoGrowthLabel({ growthStatus: "counter_correction", growthRate: null }), "计数修正 / 异常");
  assert.equal(videoGrowthLabel({ growthStatus: "zero_baseline", growthRate: null }), "基线为 0，增幅不可用");
});

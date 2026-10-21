<script setup>
import { computed, ref, watch } from "vue";
import { videoMetricGranularityLabel } from "../lib/videoMetricAggregation.js";
import { formatDecimal, formatInteger } from "../lib/numberFormatting.js";

const props = defineProps({
  title: { type: String, required: true },
  metric: { type: String, required: true },
  color: { type: String, default: "#c95c42" },
  points: { type: Array, default: () => [] },
  granularity: { type: String, default: "hour" },
});

const chartWidth = 360;
const chartHeight = 184;
const plotLeft = 46;
const plotRight = 12;
const plotTop = 16;
const plotBottom = 142;
const tickCount = 4;
const tooltipWidth = 158;
const tooltipHeight = 66;

const validMetricPoints = computed(() => props.points.filter((point) => point?.[props.metric] !== null && point?.[props.metric] !== undefined && point?.[props.metric] !== "" && Number.isFinite(Number(point[props.metric]))));
const values = computed(() => validMetricPoints.value.map((point) => Number(point[props.metric])));

const maximum = computed(() => (values.value.length ? Math.max(...values.value) : 0));
function niceStep(value) {
  if (!Number.isFinite(value) || value <= 0) return 1;
  const exponent = Math.floor(Math.log10(value));
  const fraction = value / 10 ** exponent;
  const niceFraction = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 5 ? 5 : 10;
  return niceFraction * 10 ** exponent;
}

const scaleMaximum = computed(() => {
  if (!values.value.length || maximum.value <= 0) return 1;
  return niceStep(maximum.value / tickCount) * tickCount;
});

const yTicks = computed(() =>
  Array.from({ length: tickCount + 1 }, (_, index) =>
    scaleMaximum.value - (scaleMaximum.value / tickCount) * index,
  ),
);

const pointDetails = computed(() =>
  values.value.map((value, index) => ({
    value,
    observedAt: validMetricPoints.value[index]?.observedAt,
    delta: index === 0 ? null : value - values.value[index - 1],
  })),
);

const visiblePointIndexes = computed(() => {
  const pointCount = pointDetails.value.length;
  const maxVisibleMarkers = 8;
  if (!pointCount) return new Set();
  if (pointCount <= maxVisibleMarkers) {
    return new Set(Array.from({ length: pointCount }, (_, index) => index));
  }

  const indexes = new Set([0, pointCount - 1]);
  for (let markerIndex = 1; markerIndex < maxVisibleMarkers - 1; markerIndex += 1) {
    indexes.add(Math.round((markerIndex * (pointCount - 1)) / (maxVisibleMarkers - 1)));
  }
  return indexes;
});

const granularityLabel = computed(() => videoMetricGranularityLabel(props.granularity));

const axisLabels = computed(() => {
  const lastIndex = values.value.length - 1;
  if (lastIndex < 0) return [];
  const indexes = lastIndex < 2 ? [0, lastIndex] : [0, Math.round(lastIndex / 2), lastIndex];
  return [...new Set(indexes)].map((index) => ({
    index,
    label: formatAxisTime(pointDetails.value[index]?.observedAt),
  }));
});

const activeIndex = ref(null);

watch(
  () => props.points,
  () => {
    activeIndex.value = null;
  },
  { deep: true },
);

function pointX(index) {
  const usableWidth = chartWidth - plotLeft - plotRight;
  return plotLeft + (index / Math.max(values.value.length - 1, 1)) * usableWidth;
}

function pointY(value) {
  const usableHeight = plotBottom - plotTop;
  return plotBottom - (value / scaleMaximum.value) * usableHeight;
}

const points = computed(() =>
  values.value.map((value, index) => `${pointX(index)},${pointY(value)}`).join(" "),
);

const areaPoints = computed(() => {
  if (!values.value.length) return "";
  return (
    String(plotLeft) +
    "," +
    String(plotBottom) +
    " " +
    points.value +
    " " +
    String(pointX(values.value.length - 1)) +
    "," +
    String(plotBottom)
  );
});

const activePoint = computed(() => {
  if (activeIndex.value === null) return null;
  const point = pointDetails.value[activeIndex.value];
  if (!point) return null;
  return {
    ...point,
    index: activeIndex.value,
    x: pointX(activeIndex.value),
    y: pointY(point.value),
  };
});

const tooltipBox = computed(() => {
  if (!activePoint.value) return null;
  const point = activePoint.value;
  const minX = plotLeft;
  const maxX = chartWidth - plotRight - tooltipWidth;
  const x = Math.min(Math.max(point.x - tooltipWidth / 2, minX), maxX);
  const aboveY = point.y - tooltipHeight - 10;
  const y = aboveY >= plotTop ? aboveY : point.y + 10;
  return { x, y, width: tooltipWidth, height: tooltipHeight };
});

function formatNumber(value) {
  return formatInteger(value);
}

function formatAxisNumber(value) {
  const absoluteValue = Math.abs(value);
  if (absoluteValue >= 100000000) return (value / 100000000).toFixed(1).replace(".0", "") + "亿";
  if (absoluteValue >= 10000) return (value / 10000).toFixed(1).replace(".0", "") + "万";
  if (absoluteValue >= 1000) return (value / 1000).toFixed(1).replace(".0", "") + "k";
  return formatDecimal(value);
}

function formatDelta(value) {
  if (value === null || value === undefined) return "首次观察";
  return (value > 0 ? "+" : "") + formatNumber(value);
}

function deltaLabel() {
  return props.granularity === "day" ? "较上一日增量" : "较上一小时增量";
}

function formatAxisTime(value) {
  if (!value) return "--";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "--";
  const month = String(parsed.getMonth() + 1).padStart(2, "0");
  const day = String(parsed.getDate()).padStart(2, "0");
  if (props.granularity === "day") return month + "-" + day;
  const hour = String(parsed.getHours()).padStart(2, "0");
  return month + "-" + day + " " + hour + ":00";
}

function formatObservedAt(value) {
  if (!value) return "观察时间未知";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "观察时间未知";
  return parsed.toLocaleString("zh-CN", { hour12: false });
}

function pointLabel(index) {
  const point = pointDetails.value[index];
  if (!point) return props.title;
  return (
    props.title +
    "，" +
    formatObservedAt(point.observedAt) +
    "，当前总数 " +
    formatNumber(point.value) +
    "，" +
    deltaLabel() +
    " " +
    formatDelta(point.delta)
  );
}

function setActivePoint(index) {
  activeIndex.value = index;
}

function nearestPointIndex(event) {
  const svg = event.currentTarget;
  const screenMatrix = svg?.getScreenCTM?.();
  if (!screenMatrix || !values.value.length) return null;
  const svgPoint = svg.createSVGPoint();
  svgPoint.x = event.clientX;
  svgPoint.y = event.clientY;
  const localPoint = svgPoint.matrixTransform(screenMatrix.inverse());
  if (
    localPoint.x < plotLeft ||
    localPoint.x > chartWidth - plotRight ||
    localPoint.y < plotTop ||
    localPoint.y > plotBottom
  ) {
    return null;
  }
  return values.value.reduce((nearestIndex, value, index) => {
    const nearestDistance = Math.abs(pointX(nearestIndex) - localPoint.x);
    const currentDistance = Math.abs(pointX(index) - localPoint.x);
    return currentDistance < nearestDistance ? index : nearestIndex;
  }, 0);
}

function handleChartPointer(event) {
  const index = nearestPointIndex(event);
  if (index !== null) setActivePoint(index);
}

function handleChartClick(event) {
  const index = nearestPointIndex(event);
  if (index !== null) setActivePoint(index);
}

function clearActivePoint() {
  activeIndex.value = null;
}
</script>

<template>
  <article class="video-metric-chart" :style="{ '--chart-color': color }">
    <div class="video-metric-chart-head">
      <h3>{{ title }}</h3>
      <div v-if="values.length" class="video-chart-head-meta">
        <span class="video-chart-granularity">{{ granularityLabel }}</span>
        <span class="video-chart-range">{{ formatNumber(values[values.length - 1]) }}</span>
      </div>
    </div>
    <div v-if="values.length" class="video-metric-chart-canvas">
      <svg
        :viewBox="'0 0 ' + chartWidth + ' ' + chartHeight"
        role="group"
        :aria-label="title + '历史曲线，' + granularityLabel + '，当前 ' + formatNumber(values[values.length - 1])"
        @mousemove="handleChartPointer"
        @click="handleChartClick"
        @mouseleave="clearActivePoint"
      >
        <text x="8" y="12" class="video-chart-axis-title">数量</text>
        <g v-for="tick in yTicks" :key="'tick-' + tick">
          <line
            :x1="plotLeft"
            :y1="pointY(tick)"
            :x2="chartWidth - plotRight"
            :y2="pointY(tick)"
            class="video-chart-grid-line"
          />
          <text
            :x="plotLeft - 8"
            :y="pointY(tick) + 3"
            text-anchor="end"
            class="video-chart-y-label"
          >
            {{ formatAxisNumber(tick) }}
          </text>
        </g>
        <line :x1="plotLeft" :y1="plotTop" :x2="plotLeft" :y2="plotBottom" class="video-chart-axis-line" />
        <line
          :x1="plotLeft"
          :y1="plotBottom"
          :x2="chartWidth - plotRight"
          :y2="plotBottom"
          class="video-chart-axis-line"
        />
        <polygon v-if="values.length > 1" :points="areaPoints" class="video-chart-area" />
        <polyline :points="points" class="video-chart-line" fill="none" />
        <line
          v-if="activePoint"
          :x1="activePoint.x"
          :y1="plotTop"
          :x2="activePoint.x"
          :y2="plotBottom"
          class="video-chart-active-guide"
        />
        <g v-for="(point, index) in pointDetails" :key="metric + '-' + index">
          <circle
            :cx="pointX(index)"
            :cy="pointY(point.value)"
            r="9"
            class="video-chart-point-hitbox"
            tabindex="0"
            role="button"
            :aria-label="pointLabel(index)"
            :aria-pressed="activeIndex === index"
            @focus="setActivePoint(index)"
            @keydown.enter.prevent="setActivePoint(index)"
            @keydown.space.prevent="setActivePoint(index)"
          >
            <title>{{ pointLabel(index) }}</title>
          </circle>
          <circle
            :cx="pointX(index)"
            :cy="pointY(point.value)"
            v-if="visiblePointIndexes.has(index) || activeIndex === index"
            r="3.5"
            :class="['video-chart-point', { 'is-active': activeIndex === index }]"
            aria-hidden="true"
          />
        </g>
        <g
          v-if="activePoint && tooltipBox"
          class="video-chart-tooltip"
          :transform="'translate(' + tooltipBox.x + ', ' + tooltipBox.y + ')'"
        >
          <rect :width="tooltipBox.width" :height="tooltipBox.height" rx="7" />
          <text x="10" y="17" class="video-chart-tooltip-time">{{ formatObservedAt(activePoint.observedAt) }}</text>
          <text x="10" y="36">当前总数 {{ formatNumber(activePoint.value) }}</text>
          <text x="10" y="55">{{ deltaLabel() }} {{ formatDelta(activePoint.delta) }}</text>
        </g>
      </svg>
      <div class="video-chart-axis-caption">采集时间</div>
      <div class="video-chart-axis">
        <span v-for="axisLabel in axisLabels" :key="'axis-' + axisLabel.index">{{ axisLabel.label }}</span>
      </div>
    </div>
    <div v-else class="video-metric-chart-empty">暂无已保存的历史观察</div>
  </article>
</template>

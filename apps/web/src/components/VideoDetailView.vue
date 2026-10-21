<script setup>
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { NAlert, NButton, NEmpty, NIcon, NPagination, NSelect, NSpin, NTag } from "naive-ui";
import {
  ArrowLeft,
  Bookmark,
  CalendarClock,
  Heart,
  ImageOff,
  MessageCircle,
  RefreshCw,
  Share2,
} from "lucide-vue-next";
import VideoMetricChart from "./VideoMetricChart.vue";
import AIAnalysisPanel from "./AIAnalysisPanel.vue";
import VideoCurationPanel from "./VideoCurationPanel.vue";
import { fetchVideoComments, fetchVideoDetail, fetchVideoPlayback, refreshVideoComments, refreshMonitoringMetrics } from "../lib/api.js";
import { metricsAreStale } from "../lib/autoRefresh.js";
import { formatInteger } from "../lib/numberFormatting.js";
import { aggregateVideoMetricPoints } from "../lib/videoMetricAggregation.js";
import { monitoringSourceLabel } from "../lib/monitoringTasks.js";

const props = defineProps({
  objectId: { type: String, default: "" },
});

const emit = defineEmits(["back"]);
const loading = ref(false);
const liveCollecting = ref(false);
const liveNotice = ref("");
const error = ref("");
const detail = ref(null);
const coverFailed = ref(false);
const showAI = ref(false);
let detailRequestID = 0;
const commentRefreshing = ref(false);
const commentNotice = ref("");
const commentError = ref("");
const commentsLoading = ref(false);
const commentsError = ref("");
const comments = ref({ items: [], page: 1, pageSize: 20, total: 0, totalPages: 1 });
const commentsPage = ref(1);
const commentsPageSize = ref(20);
let commentsRequestID = 0;
const playbackLoading = ref(false);
let playbackController;

const commentPageSizeOptions = [
  { label: "20 条 / 页", value: 20 },
  { label: "50 条 / 页", value: 50 },
];

const metricDefinitions = [
  { key: "likes", label: "点赞", icon: Heart, color: "#d35d55" },
  { key: "comments", label: "评论", icon: MessageCircle, color: "#537fa5" },
  { key: "favorites", label: "收藏", icon: Bookmark, color: "#b98539" },
  { key: "forwards", label: "转发", icon: Share2, color: "#5b927d" },
];

const metricCards = computed(() =>
  metricDefinitions.map((metric) => ({
    ...metric,
    value: detail.value?.metrics?.[metric.key] ?? null,
    delta: detail.value?.hasComparableInteractions ? detail.value?.positiveDeltas?.[metric.key] ?? null : null,
  })),
);

const playbackReady = computed(
  () => detail.value?.playback?.status === "ready" && detail.value?.playback?.url,
);

const chartAggregation = computed(() =>
  aggregateVideoMetricPoints(detail.value?.history || []),
);
const chartPoints = computed(() => chartAggregation.value.points);

function formatNumber(value) {
  if (value === null || value === undefined || value === "") return "不可用";
  return formatInteger(Number(value) || 0);
}

function formatDateTime(value, empty = "暂无") {
  if (!value) return empty;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return empty;
  return parsed.toLocaleString("zh-CN", { hour12: false });
}

function formatPublishedAt(value) {
  return formatDateTime(value, "发布日期未知");
}

function growthStatusLabel(status) {
  return {
    available: "可比增长",
    insufficient_history: "历史不足",
    counter_correction: "计数修正 / 异常",
    zero_baseline: "零基线 / 增长率不可用",
    metrics_unavailable: "指标缺失 / 增幅不可用",
  }[status] || "增长不可用";
}

function growthWindowLabel(window) {
  if (!window?.baselineObservedAt || !window?.latestObservedAt) return "尚未形成两个不同观察时刻";
  return `${formatDateTime(window.baselineObservedAt, "未知")} → ${formatDateTime(window.latestObservedAt, "未知")}`;
}

function resetState() {
  playbackController?.abort();
  playbackLoading.value = false;
  detail.value = null;
  error.value = "";
  coverFailed.value = false;
  commentNotice.value = "";
  commentError.value = "";
  commentsRequestID += 1;
  commentsLoading.value = false;
  commentsError.value = "";
  comments.value = { items: [], page: 1, pageSize: 20, total: 0, totalPages: 1 };
  commentsPage.value = 1;
  commentsPageSize.value = 20;
}

async function loadComments(objectID, page = commentsPage.value, pageSize = commentsPageSize.value) {
  const normalized = String(objectID || "").trim();
  if (!normalized) return;
  const requestID = ++commentsRequestID;
  commentsLoading.value = true;
  commentsError.value = "";
  try {
    const payload = await fetchVideoComments(normalized, { page, pageSize });
    if (requestID !== commentsRequestID) return;
    comments.value = {
      items: Array.isArray(payload?.items) ? payload.items : [],
      page: Number(payload?.page) || page,
      pageSize: Number(payload?.pageSize) || pageSize,
      total: Number(payload?.total) || 0,
      totalPages: Math.max(1, Number(payload?.totalPages) || 1),
    };
    commentsPage.value = comments.value.page;
    commentsPageSize.value = comments.value.pageSize;
  } catch (loadError) {
    if (requestID === commentsRequestID) {
      commentsError.value = loadError instanceof Error ? loadError.message : "视频评论加载失败";
    }
  } finally {
    if (requestID === commentsRequestID) commentsLoading.value = false;
  }
}

async function load({ background = false, collect = true } = {}) {
  if (background && (loading.value || commentRefreshing.value)) return;
  const requestID = ++detailRequestID;
  const objectId = String(props.objectId || "").trim();
  error.value = "";
  const sameVideo = detail.value?.objectId === objectId;
  const playback = sameVideo ? detail.value?.playback : null;
  if (!sameVideo) { resetState(); liveNotice.value = ""; }
  if (!objectId) {
    error.value = "缺少视频标识，无法打开详情。";
    return;
  }
  if (!background) loading.value = true;
  try {
    const payload = await fetchVideoDetail(objectId);
    if (requestID !== detailRequestID) return;
    detail.value = payload;
    if (playback) detail.value.playback = playback;
    void loadComments(objectId, background ? commentsPage.value : 1, commentsPageSize.value);
    if (!background || !playback) void loadPlayback();
    if (collect && metricsAreStale(payload.lastObservedAt)) void collectLiveMetrics();
  } catch (loadError) {
    if (requestID === detailRequestID) error.value = loadError instanceof Error ? loadError.message : "视频详情加载失败";
  } finally {
    if (requestID === detailRequestID) loading.value = false;
  }
}

async function collectLiveMetrics(force = false) {
  if (liveCollecting.value || !detail.value) return;
  const id = props.objectId;
  liveCollecting.value = true;
  liveNotice.value = "正在采集最新互动指标，当前数据可继续查看…";
  try {
    const result = await refreshMonitoringMetrics("video", id, force);
    if (props.objectId !== id) return;
    liveNotice.value = result.message || "指标仍在 10 分钟有效期内";
    if (result.status === "updated") await load({ background: true, collect: false });
  } catch (cause) {
    if (props.objectId === id) liveNotice.value = `实时采集失败，保留上次有效数据：${cause instanceof Error ? cause.message : cause}`;
  } finally { liveCollecting.value = false; }
}

async function loadPlayback() {
  if (!detail.value || playbackLoading.value) return;
  const requestID = detailRequestID;
  const objectID = props.objectId;
  playbackController?.abort();
  const controller = new AbortController();
  playbackController = controller;
  playbackLoading.value = true;
  const timer = setTimeout(() => controller.abort(), 10000);
  try {
    const playback = await fetchVideoPlayback(objectID, controller.signal);
    if (requestID === detailRequestID && detail.value) detail.value.playback = playback;
  } catch {
    if (requestID === detailRequestID && detail.value) detail.value.playback = { status: "unavailable", message: "预览暂不可用，检查采集器连接后可重试。历史数据分析不受影响。" };
  } finally {
    clearTimeout(timer);
    if (requestID === detailRequestID) playbackLoading.value = false;
  }
}

async function refreshComments() {
  const objectId = String(props.objectId || "").trim();
  if (!objectId || commentRefreshing.value) return;
  commentRefreshing.value = true;
  commentNotice.value = "";
  commentError.value = "";
  try {
    const payload = await refreshVideoComments(objectId);
    await loadComments(objectId, 1, commentsPageSize.value);
    commentNotice.value = payload?.status === "success" ? "该视频评论已提交采集。" : "该视频评论采集已返回结果，请查看状态。";
  } catch (actionError) {
    commentError.value = actionError instanceof Error ? actionError.message : "视频评论采集失败";
  } finally {
    commentRefreshing.value = false;
  }
}

function handleCommentPageChange(page) {
  void loadComments(props.objectId, page, commentsPageSize.value);
}

function handleCommentPageSizeChange(pageSize) {
  commentsPageSize.value = pageSize;
  void loadComments(props.objectId, 1, pageSize);
}

function commentAuthor(comment) {
  return comment?.authorNickname || comment?.authorUsername || "匿名用户";
}

function compactIdentifier(value) {
  const normalized = String(value || "").trim();
  if (!normalized) return "";
  if (normalized.length <= 26) return normalized;
  return `${normalized.slice(0, 14)}…${normalized.slice(-8)}`;
}

function commentAuthorID(comment) {
  const username = String(comment?.authorUsername || "").trim();
  return username ? `@${compactIdentifier(username)}` : "";
}

function commentAuthorIDTitle(comment) {
  const username = String(comment?.authorUsername || "").trim();
  return username ? `完整评论者 ID：@${username}` : "";
}

function commentSource(source) {
  return {
    wx_comment_export: "评论导出",
    feed_comment_list: "评论列表",
    channel_feed_pull: "评论列表",
    edge_fetch_feed_comment_list: "评论采集",
    wx_video_profile: "视频指标采集",
  }[source] || source || "来源待确认";
}

function handleCoverError() {
  coverFailed.value = true;
}

watch(() => props.objectId, load, { immediate: true });
onBeforeUnmount(() => { detailRequestID += 1; playbackController?.abort(); });
defineExpose({ refresh: load });
</script>

<template>
  <section class="video-detail-page" aria-labelledby="video-detail-title">
    <div class="video-detail-toolbar">
      <n-button secondary @click="emit('back')">
        <template #icon>
          <n-icon><ArrowLeft /></n-icon>
        </template>
        返回上一页
      </n-button>
      <n-button secondary :loading="loading" @click="load">
        <template #icon>
          <n-icon><RefreshCw /></n-icon>
        </template>
        刷新详情
      </n-button>
      <n-button secondary :loading="liveCollecting" @click="collectLiveMetrics(true)">立即采集指标</n-button>
      <n-button secondary :loading="commentRefreshing" @click="refreshComments">
        <template #icon>
          <n-icon><MessageCircle /></n-icon>
        </template>
        获取评论
      </n-button>
    </div>
    <n-alert v-if="liveNotice" type="info" show-icon>{{ liveNotice }}</n-alert>

    <div v-if="loading" class="video-detail-loading">
      <n-spin size="medium" />
      <span>正在读取视频详情和历史数据…</span>
    </div>

    <n-alert v-else-if="error && !detail" type="error" show-icon>
      {{ error }}
    </n-alert>

    <template v-else-if="detail">
      <n-alert v-if="error" type="warning" show-icon>{{ error }}。上次有效数据仍可查看。</n-alert>
      <n-alert v-if="commentNotice" type="success" show-icon closable @close="commentNotice = ''">
        {{ commentNotice }}
      </n-alert>
      <n-alert v-if="commentError" type="error" show-icon closable @close="commentError = ''">{{ commentError }}
      </n-alert>
      <VideoCurationPanel :object-id="objectId" :title="detail.title" />
      <section class="video-detail-hero">
        <div class="video-detail-media">
          <video
            v-if="playbackReady"
            class="video-detail-player"
            controls
            preload="metadata"
            :poster="detail.coverUrl || undefined"
            :src="detail.playback.url"
          ></video>
          <div v-else class="video-detail-cover-state">
            <img
              v-if="detail.coverUrl && !coverFailed"
              :src="detail.coverUrl"
              :alt="`${detail.title || '视频'}封面`"
              @error="handleCoverError"
            />
            <div v-else class="video-detail-cover-empty">
              <n-icon size="24"><ImageOff /></n-icon>
              <span>暂无封面</span>
            </div>
            <div class="video-detail-playback-status">
              <strong>{{ playbackLoading ? '正在获取视频预览…' : '暂不可播放' }}</strong>
              <span>{{ playbackLoading ? '历史指标已就绪，可继续查看分析。' : detail.playback.message || "没有可用的临时播放地址。" }}</span>
              <button v-if="!playbackLoading" class="tool-button secondary" type="button" @click="loadPlayback">重试视频预览</button>
            </div>
          </div>
        </div>

        <div class="video-detail-hero-copy">
          <div class="detail-title-row">
            <n-tag size="small" :bordered="false" type="info">视频详情</n-tag>
            <span v-if="playbackReady" class="status-badge" data-tone="good">可播放</span>
            <span v-else class="status-badge" data-tone="warn">{{ playbackLoading ? '预览加载中' : '预览暂不可用' }}</span>
          </div>
          <h2 id="video-detail-title">{{ detail.title || "未命名视频" }}</h2>
          <p class="video-detail-channel">
            {{ detail.channelName || detail.channelUsername || "账号待确认" }}
            <span v-if="detail.channelUsername"> · {{ detail.channelUsername }}</span>
          </p>
          <div class="video-detail-meta">
            <span><n-icon><CalendarClock /></n-icon> 发布于 {{ formatPublishedAt(detail.publishedAt) }}</span>
            <span>最近观察 {{ formatDateTime(detail.lastObservedAt) }}</span>
            <span>来源 {{ detail.source ? monitoringSourceLabel(detail.source) : "来源待确认" }}</span>
          </div>
          <div v-if="detail.sourceKeywords?.length" class="video-detail-provenance">
            <span class="video-detail-provenance-label">来源关键词</span>
            <span v-for="keyword in detail.sourceKeywords" :key="keyword" class="chip">{{ keyword }}</span>
          </div>
          <p class="video-detail-basis">{{ detail.dataBasis?.replaceAll('wx_channel', '视频号') }}</p>
          <div v-if="detail.tags?.length" class="chip-row">
            <span v-for="tag in detail.tags" :key="tag" class="chip">{{ tag }}</span>
          </div>
        </div>
      </section>

      <section class="video-detail-section" aria-labelledby="video-current-metrics-title">
        <div class="video-detail-section-head">
          <div>
            <div class="panel-eyebrow">当前数据</div>
            <h2 id="video-current-metrics-title">四项互动指标</h2>
          </div>
          <span class="video-detail-observation-state">
            {{ detail.observationCount }} 次观察 · {{ growthStatusLabel(detail.growthStatus) }}
          </span>
        </div>
        <div class="video-detail-metric-grid">
          <article v-for="metric in metricCards" :key="metric.key" class="video-detail-metric-card">
            <div class="video-detail-metric-label">
              <n-icon :style="{ color: metric.color }"><component :is="metric.icon" /></n-icon>
              <span>{{ metric.label }}</span>
            </div>
            <strong>{{ formatNumber(metric.value) }}</strong>
            <small>{{ detail.hasComparableInteractions ? `最近两次新增 ${formatNumber(metric.delta)}` : growthStatusLabel(detail.growthStatus) }}</small>
          </article>
        </div>
      </section>

      <section class="video-detail-section" aria-labelledby="video-history-title">
        <div class="video-detail-section-head">
          <div>
            <div class="panel-eyebrow">历史变化</div>
            <h2 id="video-history-title">互动数据曲线</h2>
          </div>
          <span class="video-detail-observation-state">
            不含播放/阅读数 · {{ chartAggregation.label }}
            <template v-if="chartAggregation.rawCount > chartPoints.length">
              · {{ chartAggregation.rawCount }} 次观察压缩为 {{ chartPoints.length }} 个点
            </template>
          </span>
        </div>
        <div class="video-detail-growth-evidence">
          <strong>增长依据：</strong>{{ detail.growthBasis || "仅基于点赞、评论、收藏、转发。" }}
          <span>观察窗口：{{ growthWindowLabel(detail.growthWindow) }}</span>
        </div>
        <div v-if="chartPoints.length" class="video-metric-chart-grid">
          <VideoMetricChart
            v-for="metric in metricDefinitions"
            :key="metric.key"
            :title="metric.label"
            :metric="metric.key"
            :color="metric.color"
            :points="chartPoints"
            :granularity="chartAggregation.granularity"
          />
        </div>
        <n-empty v-else description="暂无已保存的历史观察，当前只展示实时视频资料。" />
      </section>

      <section class="video-detail-section video-comments-section" aria-labelledby="video-comments-title">
        <div class="video-detail-section-head">
          <div>
            <div class="panel-eyebrow">原始评论</div>
            <h2 id="video-comments-title">视频评论</h2>
          </div>
          <div class="video-comments-section-tools">
            <span class="video-comment-binding" :title="detail.objectId || props.objectId">已绑定当前视频</span>
            <n-select
              :value="commentsPageSize"
              :options="commentPageSizeOptions"
              size="small"
              class="video-comments-page-size"
              aria-label="评论每页条数"
              @update:value="handleCommentPageSizeChange"
            />
          </div>
        </div>

        <n-alert v-if="commentsError" type="warning" show-icon>
          {{ commentsError }}
        </n-alert>
        <div v-if="commentsLoading" class="video-comments-loading">
          <n-spin size="small" />
          <span>正在读取当前视频评论</span>
        </div>
        <n-empty v-else-if="!commentsError && !comments.items.length" description="当前视频暂无已采集评论" />
        <div v-else-if="comments.items.length" class="video-comments-table-wrap">
          <table class="video-comments-table">
            <thead>
              <tr>
                <th>评论内容</th>
                <th>评论者</th>
                <th>发布时间</th>
                <th>互动</th>
                <th>地区</th>
                <th>来源</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="comment in comments.items" :key="comment.id || comment.commentId">
                <td>
                  <div class="video-comment-content" :class="{ 'is-reply': comment.parentCommentId }">
                    <span v-if="comment.parentCommentId" class="video-comment-reply-mark">回复</span>
                    <span :title="comment.content || '暂无文字内容'">{{ comment.content || "暂无文字内容" }}</span>
                  </div>
                </td>
                <td class="video-comment-author-cell">
                  <div class="video-comment-author-block">
                    <strong class="video-comment-author" :title="commentAuthor(comment)">{{ commentAuthor(comment) }}</strong>
                    <span
                      v-if="comment.authorUsername"
                      class="video-comment-author-id"
                      :title="commentAuthorIDTitle(comment)"
                    >
                      <span class="video-comment-author-id-label">ID</span>
                      <span class="video-comment-author-id-value">{{ commentAuthorID(comment) }}</span>
                    </span>
                    <span v-else class="video-comment-author-id is-empty">ID 未提供</span>
                  </div>
                </td>
                <td class="video-comment-time">{{ formatDateTime(comment.createdAt || comment.collectedAt, "时间未知") }}</td>
                <td class="video-comment-interaction-cell">
                  <span class="video-comment-interaction">赞 {{ formatNumber(comment.likeCount) }}</span>
                  <span class="video-comment-interaction">回复 {{ formatNumber(comment.replyCount) }}</span>
                </td>
                <td>{{ comment.ipRegion || "—" }}</td>
                <td>
                  <span class="video-comment-source" :title="comment.source || '来源待确认'">{{ commentSource(comment.source) }}</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-if="comments.total > 0" class="video-comments-pagination">
          <span>共 {{ formatNumber(comments.total) }} 条</span>
          <n-pagination
            :page="commentsPage"
            :page-count="comments.totalPages"
            :page-size="commentsPageSize"
            size="small"
            @update:page="handleCommentPageChange"
          />
        </div>
      </section>
      <section class="video-detail-section" aria-label="可选数据解释">
        <button class="tool-button secondary" type="button" @click="showAI = !showAI">{{ showAI ? '收起' : '展开' }}可选 AI 数据解释</button>
        <p>指标和历史来自采集事实。AI 仅提供判断与待验证项，不能替代事实；未配置 AI 不影响本页分析。</p>
        <AIAnalysisPanel v-if="showAI" module="content-analysis" :scope-key="`video:${objectId}`" :variables="{ video_json: detail, data_json: { metrics: detail.metrics, history: detail.history, commentSampleCount: comments.total } }" title="视频表现数据解释" />
      </section>
    </template>
  </section>
</template>

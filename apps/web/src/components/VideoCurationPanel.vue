<script setup>
import { ref, watch } from 'vue';
import { NAlert } from 'naive-ui';
import { BookmarkCheck } from 'lucide-vue-next';
import { curateVideos, fetchVideoCuration } from '../lib/api.js';
import { curationLabel } from '../lib/videoWorkbench.js';
import VideoCurationEditor from './VideoCurationEditor.vue';
const props = defineProps({ objectId: { type: String, required: true }, title: { type: String, default: '' } });
const curation = ref(null); const busy = ref(false); const loading = ref(false); const error = ref(''); const notice = ref(''); const editorOpen = ref(false);
let request = 0;
async function load() {
  const id = ++request; loading.value = true;
  try { const value = await fetchVideoCuration(`video:${props.objectId}`); if (id === request) { curation.value = value; error.value = ''; } }
  catch (cause) { if (id === request) error.value = `运营标记读取失败：${cause.message}`; }
  finally { if (id === request) loading.value = false; }
}
async function mark(status) {
  if (busy.value) return;
  const objectId = props.objectId; busy.value = true; error.value = ''; notice.value = '';
  try { await curateVideos({ ids: [`video:${objectId}`], status }); if (objectId === props.objectId) { notice.value = `已设为${curationLabel(status)}，原始采集数据已保留。`; await load(); } }
  catch (cause) { if (objectId === props.objectId) error.value = `标记失败：${cause.message}，请重试。`; }
  finally { busy.value = false; }
}
async function saved() { notice.value = '运营标记与复盘备注已保存。'; await load(); }
watch(() => props.objectId, () => { curation.value = null; notice.value = ''; error.value = ''; editorOpen.value = false; load(); }, { immediate: true });
</script>
<template><section class="analysis-surface ops-video-curation-panel" aria-label="视频运营管理"><div class="ops-section-heading"><div><h3>运营管理 <span class="ops-curation-badge" :data-status="curation?.status">{{ loading ? '读取中…' : curationLabel(curation?.status) }}</span></h3><p>留下优秀内容，记录可复用经验；排除的内容可以恢复。</p></div><div class="ops-row-actions"><button class="tool-button secondary" type="button" :disabled="busy || loading || !curation" @click="mark('selected')"><BookmarkCheck :size="14" />加入精选</button><button class="tool-button secondary" type="button" :disabled="busy || loading || !curation" @click="mark('watching')">待观察</button><button class="tool-button secondary" type="button" :disabled="busy || loading || !curation" @click="mark(curation?.status === 'excluded' ? 'unreviewed' : 'excluded')">{{ curation?.status === 'excluded' ? '恢复内容' : '排除内容' }}</button><button class="tool-button secondary" type="button" :disabled="busy || loading || !curation" @click="editorOpen = true">标签与备注</button></div></div><n-alert v-if="error" type="error">{{ error }}<button type="button" class="operations-text-button" @click="load">重试</button></n-alert><p v-if="notice" role="status">{{ notice }}</p><div v-if="curation?.tags?.length" class="ops-video-tags"><span v-for="tag in curation.tags" :key="tag" class="ops-content-tag">{{ tag }}</span></div><p v-if="curation?.note" class="ops-video-note">{{ curation.note }}</p><VideoCurationEditor v-model:show="editorOpen" :videos="[{ id: `video:${objectId}`, title, curation }]" @saved="saved" /></section></template>

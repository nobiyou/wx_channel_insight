<script setup>
import { ref, watch } from 'vue';
import { NAlert, NModal } from 'naive-ui';
import { curateVideos } from '../lib/api.js';
import { VIDEO_CURATION_STATES } from '../lib/videoWorkbench.js';
const props = defineProps({ show: Boolean, videos: { type: Array, default: () => [] } });
const emit = defineEmits(['update:show', 'saved']);
const status = ref(''); const note = ref(''); const tags = ref(''); const busy = ref(false); const error = ref('');
watch(() => props.show, show => {
  if (!show) return;
  const single = props.videos.length === 1 ? props.videos[0] : null;
  status.value = single?.curation?.status || ''; note.value = single?.curation?.note || ''; tags.value = single?.curation?.tags?.join('，') || ''; error.value = '';
});
async function save() {
  if (busy.value) return;
  const labels = [...new Set(tags.value.split(/[,，\n]/).map(tag => tag.trim()).filter(Boolean))];
  if (labels.length > 12 || labels.some(tag => [...tag].length > 30)) { error.value = '最多 12 个标签，每个不超过 30 字。'; return; }
  busy.value = true; error.value = '';
  try {
    await curateVideos({ ids: props.videos.map(video => video.id), tags: labels, ...(status.value ? { status: status.value } : {}), ...(props.videos.length === 1 ? { note: note.value } : {}) });
    emit('saved', props.videos.length); emit('update:show', false);
  } catch (cause) { error.value = `保存失败：${cause.message}。输入已保留，请重试。`; }
  finally { busy.value = false; }
}
</script>
<template><n-modal :show="show" preset="card" :title="videos.length === 1 ? '运营标记与复盘备注' : `批量设置标签 · ${videos.length} 条视频`" class="ops-curation-modal" :mask-closable="!busy" :closable="!busy" @update:show="emit('update:show', $event)"><form class="ops-curation-form" @submit.prevent="save"><p v-if="videos.length === 1" class="ops-editor-video-title">{{ videos[0]?.title || videos[0]?.id }}</p><p v-else>本次标签会替换所选视频原有标签，备注会保留。</p><n-alert v-if="error" type="error">{{ error }}</n-alert><label><span>运营状态</span><select v-model="status" :disabled="busy"><option v-if="videos.length > 1" value="">保留各视频当前状态</option><option v-for="item in VIDEO_CURATION_STATES" :key="item.value" :value="item.value">{{ item.label }}</option></select></label><label><span>内容标签</span><input v-model="tags" :disabled="busy" placeholder="例如：高转发、开场参考、情绪共鸣（逗号分隔）" /><small>最多 12 个标签，每个 30 字。留空会清除标签。</small></label><label v-if="videos.length === 1"><span>复盘备注</span><textarea v-model="note" :disabled="busy" maxlength="2000" rows="5" placeholder="记录值得学习的开场、表达方式、受众反馈，或排除原因"></textarea></label><div class="ops-editor-actions"><button class="tool-button secondary" type="button" :disabled="busy" @click="emit('update:show', false)">取消</button><button class="tool-button primary" type="submit" :disabled="busy">{{ busy ? '保存中…' : '保存标记' }}</button></div></form></n-modal></template>

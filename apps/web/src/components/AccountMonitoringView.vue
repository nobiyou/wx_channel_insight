<script setup>
import { ref } from "vue";
import AccountLibraryView from "./AccountLibraryView.vue";
import MonitoringTasksPanel from "./MonitoringTasksPanel.vue";
import MonitoringObjectForm from "./MonitoringObjectForm.vue";
import OperationsPortfolio from "./OperationsPortfolio.vue";
import { discoveryPreset } from "../lib/videoWorkbench.js";
import { formatInteger } from "../lib/numberFormatting.js";
const props = defineProps({ bindingStates: { type: Array, default: () => [] }, baseUrl: { type: String, default: "" }, clientId: { type: String, default: "" } });
const emit = defineEmits(["open-detail", "refresh", "analyze"]);
const library = ref(null);
const tasks = ref(null);
const portfolio = ref(null);
const formatNumber = (value) => formatInteger(Number(value) || 0);
const formatDateTime = (value) => value && Number.isFinite(new Date(value).getTime()) ? new Date(value).toLocaleString("zh-CN", { hour12: false }) : "不可用";
async function saved() { emit("refresh"); await Promise.allSettled([library.value?.refresh(), tasks.value?.refresh(), portfolio.value?.refresh()]); }
async function tasksChanged() { emit("refresh"); await library.value?.refresh(); }
defineExpose({ refresh: () => Promise.allSettled([library.value?.refresh(), tasks.value?.refresh(), portfolio.value?.refresh()]) });
</script>
<template>
  <div class="operations-core-page">
    <OperationsPortfolio ref="portfolio" group-by="account" @filter="emit('analyze', $event)" @discover="emit('analyze', discoveryPreset({}, $event))" />
    <MonitoringObjectForm mode="accounts" :base-url="baseUrl" :client-id="clientId" @saved="saved" />
    <AccountLibraryView ref="library" :format-number="formatNumber" :format-date-time="formatDateTime" @open-detail="emit('open-detail', $event)" @changed="saved" />
    <MonitoringTasksPanel ref="tasks" scope="accounts" :binding-states="bindingStates" @changed="tasksChanged" />
  </div>
</template>

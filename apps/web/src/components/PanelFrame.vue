<script setup>
defineProps({
  title: String,
  eyebrow: String,
  action: String
})
</script>

<template>
  <section class="panel-frame">
    <div class="panel-head">
      <div>
        <div v-if="eyebrow" class="panel-eyebrow">{{ eyebrow }}</div>
        <h2 class="panel-title">{{ title }}</h2>
      </div>
      <div v-if="action" class="panel-action">{{ action }}</div>
    </div>
    <div class="panel-body">
      <slot />
    </div>
  </section>
</template>


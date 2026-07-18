<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="机器人管理"></n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <BotContent />
    </n-card>
  </div>
</template>

<script lang="ts" setup>
  import BotContent from './content.vue';
</script>

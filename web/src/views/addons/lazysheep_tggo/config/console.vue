<template>
  <n-tabs type="line" animated>
    <n-tab-pane name="bot" tab="机器人管理">
      <BotManage />
    </n-tab-pane>
    <n-tab-pane name="config" tab="插件配置">
      <PluginConfig />
    </n-tab-pane>
    <n-tab-pane name="global" tab="全局配置">
      <GlobalConfig />
    </n-tab-pane>
    <n-tab-pane name="monitor" tab="拉取监控">
      <PullMonitor />
    </n-tab-pane>
  </n-tabs>
</template>

<script lang="ts" setup>
  import BotManage from '../bot/content.vue';
  import PluginConfig from './content.vue';
  import GlobalConfig from '../global/index.vue';
  import PullMonitor from './monitor.vue';
</script>

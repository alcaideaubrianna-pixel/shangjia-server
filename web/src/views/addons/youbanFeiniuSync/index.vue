<template>
  <div>
    <div class="n-layout-page-header">
      <n-card :bordered="false" title="FeiNiu同步">
        <span>定时增量同步 FeiNiu 频道、资料和媒体到悦伴上架系统</span>
      </n-card>
    </div>
    <n-card :bordered="false" class="proCard">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <n-tab-pane name="dashboard" tab="监控大盘"><DashboardPanel /></n-tab-pane>
        <n-tab-pane name="configs" tab="同步配置"><ConfigPanel /></n-tab-pane>
        <n-tab-pane name="channels" tab="频道映射"><ChannelPanel /></n-tab-pane>
        <n-tab-pane name="runs" tab="运行记录"><RunPanel /></n-tab-pane>
      </n-tabs>
    </n-card>
  </div>
</template>

<script setup lang="ts">
  import { ref } from 'vue';
  import DashboardPanel from './components/dashboard-panel.vue';
  import ConfigPanel from './components/config-panel.vue';
  import ChannelPanel from './components/channel-panel.vue';
  import RunPanel from './components/run-panel.vue';

  const activeTab = ref('dashboard');
</script>
